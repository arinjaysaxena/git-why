// Package cache is a small on-disk key/value store used to remember forge
// responses between runs. Entries carry the ETag the server sent, so a later
// request can revalidate with If-None-Match instead of spending rate limit.
//
// A nil *Cache is valid and behaves as an always-empty cache that discards
// writes, so callers need no "is caching enabled" branches.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Entry is one cached response.
type Entry struct {
	// ETag is the validator to send back in If-None-Match; may be empty.
	ETag string `json:"etag,omitempty"`
	// Body is the response payload.
	Body []byte `json:"body"`
	// Stored is when the entry was written.
	Stored time.Time `json:"stored"`
}

// Cache stores entries as one JSON file per key under Dir.
type Cache struct {
	dir string
	now func() time.Time
}

// envDir overrides the cache location.
const envDir = "GIT_WHY_CACHE"

// DefaultDir returns the cache directory: $GIT_WHY_CACHE, else
// $XDG_CACHE_HOME/git-why, else the OS user cache directory plus "git-why".
func DefaultDir() (string, error) {
	if d := os.Getenv(envDir); d != "" {
		return d, nil
	}
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return filepath.Join(d, "git-why"), nil
	}
	d, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("cache: locate user cache directory: %w", err)
	}
	return filepath.Join(d, "git-why"), nil
}

// Open returns a cache rooted at dir. The directory is created lazily on the
// first write, so opening never fails and never touches the disk.
func Open(dir string) *Cache { return &Cache{dir: dir, now: time.Now} }

// Dir reports the cache directory ("" for a nil cache).
func (c *Cache) Dir() string {
	if c == nil {
		return ""
	}
	return c.dir
}

type record struct {
	Key string `json:"key"`
	Entry
}

func (c *Cache) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	h := hex.EncodeToString(sum[:])
	return filepath.Join(c.dir, h[:2], h+".json")
}

// Get returns the entry stored under key. Unreadable or corrupt files count
// as a miss and are removed so they cannot poison later runs.
func (c *Cache) Get(key string) (Entry, bool) {
	if c == nil {
		return Entry{}, false
	}
	p := c.path(key)
	data, err := os.ReadFile(p)
	if err != nil {
		return Entry{}, false
	}
	var r record
	if err := json.Unmarshal(data, &r); err != nil || r.Key != key {
		_ = os.Remove(p)
		return Entry{}, false
	}
	return r.Entry, true
}

// Put stores e under key, replacing any previous entry. The write is atomic:
// readers see either the old entry or the new one, never a partial file.
func (c *Cache) Put(key string, e Entry) error {
	if c == nil {
		return nil
	}
	if e.Stored.IsZero() {
		e.Stored = c.now().UTC()
	}
	data, err := json.Marshal(record{Key: key, Entry: e})
	if err != nil {
		return fmt.Errorf("cache: encode entry: %w", err)
	}
	p := c.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("cache: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return fmt.Errorf("cache: %w", err)
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("cache: write entry: %w", err)
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("cache: %w", err)
	}
	return nil
}

// Stats summarises the cache contents.
type Stats struct {
	Entries int
	Bytes   int64
	// Scopes counts entries per key scope (the first three "/" segments of the
	// key, e.g. "github.com/owner/repo").
	Scopes map[string]int
	Oldest time.Time
	Newest time.Time
}

// Stats scans the cache directory. A missing directory is an empty cache.
func (c *Cache) Stats() (Stats, error) {
	s := Stats{Scopes: map[string]int{}}
	if c == nil {
		return s, nil
	}
	err := c.walk(func(path string, size int64) {
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		var r record
		if json.Unmarshal(data, &r) != nil {
			return
		}
		s.Entries++
		s.Bytes += size
		s.Scopes[scope(r.Key)]++
		if s.Oldest.IsZero() || r.Stored.Before(s.Oldest) {
			s.Oldest = r.Stored
		}
		if r.Stored.After(s.Newest) {
			s.Newest = r.Stored
		}
	})
	return s, err
}

// Clear deletes every entry and returns how many were removed. Only files
// this package wrote are touched; the directory itself is kept.
func (c *Cache) Clear() (int, error) {
	if c == nil {
		return 0, nil
	}
	n := 0
	var errs []error
	err := c.walk(func(path string, _ int64) {
		if err := os.Remove(path); err != nil {
			errs = append(errs, err)
			return
		}
		n++
	})
	errs = append(errs, err)
	// Drop the now-empty shard directories; failure just means something is left in them.
	if shards, err := os.ReadDir(c.dir); err == nil {
		for _, sh := range shards {
			if sh.IsDir() && len(sh.Name()) == 2 {
				_ = os.Remove(filepath.Join(c.dir, sh.Name()))
			}
		}
	}
	return n, errors.Join(errs...)
}

// walk calls fn for every entry file (<dir>/<2 hex>/<64 hex>.json).
func (c *Cache) walk(fn func(path string, size int64)) error {
	shards, err := os.ReadDir(c.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cache: %w", err)
	}
	for _, sh := range shards {
		if !sh.IsDir() || len(sh.Name()) != 2 {
			continue
		}
		files, err := os.ReadDir(filepath.Join(c.dir, sh.Name()))
		if err != nil {
			return fmt.Errorf("cache: %w", err)
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") || len(f.Name()) != 69 {
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			fn(filepath.Join(c.dir, sh.Name(), f.Name()), info.Size())
		}
	}
	return nil
}

func scope(key string) string {
	parts := strings.SplitN(key, "/", 4)
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return strings.Join(parts, "/")
}
