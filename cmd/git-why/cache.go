package main

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/arinjaysaxena/git-why/internal/cache"
)

// newCacheCmd builds `git why cache stats|clear`.
func newCacheCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Inspect or clear the on-disk response cache",
		Long: "git-why caches forge responses under $GIT_WHY_CACHE, $XDG_CACHE_HOME/git-why or the\n" +
			"OS user cache directory, and revalidates them with ETags.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usagef("unknown cache command %q (use stats or clear)", args[0])
			}
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "stats",
			Short: "Show how many entries are cached and how large the cache is",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				c, err := openCache()
				if err != nil {
					return err
				}
				s, err := c.Stats()
				if err != nil {
					return err
				}
				writeStats(cmd.OutOrStdout(), c.Dir(), s)
				return nil
			},
		},
		&cobra.Command{
			Use:   "clear",
			Short: "Delete every cached entry",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				c, err := openCache()
				if err != nil {
					return err
				}
				n, err := c.Clear()
				if err != nil {
					return fmt.Errorf("cache clear: %w", err)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "removed %d %s from %s\n", n, plural(n, "entry", "entries"), c.Dir())
				return nil
			},
		},
	)
	return cmd
}

func openCache() (*cache.Cache, error) {
	dir, err := cache.DefaultDir()
	if err != nil {
		return nil, err
	}
	return cache.Open(dir), nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func writeStats(w io.Writer, dir string, s cache.Stats) {
	_, _ = fmt.Fprintf(w, "cache:   %s\nentries: %d\nsize:    %s\n", dir, s.Entries, humanBytes(s.Bytes))
	if s.Entries == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "oldest:  %s\nnewest:  %s\n", s.Oldest.Local().Format(time.RFC3339), s.Newest.Local().Format(time.RFC3339))
	scopes := make([]string, 0, len(s.Scopes))
	for k := range s.Scopes {
		scopes = append(scopes, k)
	}
	sort.Strings(scopes)
	_, _ = fmt.Fprintln(w, "repositories:")
	for _, k := range scopes {
		_, _ = fmt.Fprintf(w, "  %-40s %d\n", k, s.Scopes[k])
	}
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGT"[exp])
}
