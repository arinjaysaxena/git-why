// Package evidence assembles the commits, pull requests, issues and review
// comments found for a line range into one typed graph that ranking,
// narration and the JSON output all consume.
package evidence

import (
	"regexp"
	"strconv"
	"strings"
)

// Ref is a reference to an issue or pull request found in free text.
type Ref struct {
	// Number is the issue or PR number.
	Number int `json:"number"`
	// Owner and Repo are empty for references to the current repository.
	Owner string `json:"owner,omitempty"`
	Repo  string `json:"repo,omitempty"`
	// Kind is "issue" or "pr" when the form of the reference says so (a URL
	// containing /issues/ or /pull/), and empty for ambiguous "#123" forms.
	Kind string `json:"kind,omitempty"`
	// Closes is true when a closing keyword (fixes, closes, resolves, ...)
	// directly precedes the reference.
	Closes bool `json:"closes,omitempty"`
	// Raw is the text as written.
	Raw string `json:"raw"`
}

// Local reports whether the reference points at the repository owner/repo
// (references without an explicit repository are always local).
func (r Ref) Local(owner, repo string) bool {
	if r.Owner == "" && r.Repo == "" {
		return true
	}
	return strings.EqualFold(r.Owner, owner) && strings.EqualFold(r.Repo, repo)
}

var (
	// https://host/owner/repo/issues/12 or /pull/12 (optionally followed by a #fragment).
	urlRef = regexp.MustCompile(`https?://[A-Za-z0-9.-]+(?::\d+)?/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/(issues|pull)/(\d+)(?:#[\w-]*)?`)
	// owner/repo#12
	crossRef = regexp.MustCompile(`(^|[^\w/.-])([A-Za-z0-9][A-Za-z0-9_.-]*)/([A-Za-z0-9_.-]+)#(\d+)\b`)
	// #12 and GH-12 on a word boundary (not part of a URL fragment, hex colour or identifier).
	shortRef = regexp.MustCompile(`(^|[^\w/&#-])(?:#|(?i:GH-))(\d+)\b`)
	// A closing keyword immediately before a reference.
	closeWord = regexp.MustCompile(`(?i)\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s*:?\s*$`)
)

// maxRefNumber guards against absurd numbers such as long digit runs in logs.
const maxRefNumber = 99_999_999

// ParseRefs finds issue and pull request references in text, in order of
// appearance, without duplicates. It understands "#123", "GH-123",
// "owner/repo#123" and issue/PR URLs; URLs are consumed first so their
// fragments are never misread as short references.
func ParseRefs(text string) []Ref {
	type hit struct {
		pos int
		ref Ref
	}
	var hits []hit
	masked := []byte(text)
	mask := func(start, end int) {
		for i := start; i < end; i++ {
			masked[i] = ' '
		}
	}
	closes := func(before string) bool { return closeWord.MatchString(before) }

	for _, m := range urlRef.FindAllStringSubmatchIndex(text, -1) {
		n, ok := number(text[m[8]:m[9]])
		if ok {
			kind := "issue"
			if text[m[6]:m[7]] == "pull" {
				kind = "pr"
			}
			hits = append(hits, hit{m[0], Ref{Number: n, Owner: text[m[2]:m[3]], Repo: text[m[4]:m[5]], Kind: kind, Closes: closes(text[:m[0]]), Raw: text[m[0]:m[1]]}})
		}
		mask(m[0], m[1])
	}
	cur := string(masked)
	for _, m := range crossRef.FindAllStringSubmatchIndex(cur, -1) {
		if n, ok := number(cur[m[8]:m[9]]); ok {
			start := m[4] // start of owner
			hits = append(hits, hit{start, Ref{Number: n, Owner: cur[m[4]:m[5]], Repo: cur[m[6]:m[7]], Closes: closes(text[:start]), Raw: text[start:m[1]]}})
		}
		mask(m[4], m[1])
	}
	cur = string(masked)
	for _, m := range shortRef.FindAllStringSubmatchIndex(cur, -1) {
		if n, ok := number(cur[m[4]:m[5]]); ok {
			start := m[3] // end of the leading-boundary group is where "#12" / "GH-12" begins
			hits = append(hits, hit{start, Ref{Number: n, Closes: closes(text[:start]), Raw: text[start:m[1]]}})
		}
	}

	// Order by position, then drop repeats of the same target (a later mention
	// can still upgrade an earlier one to "closes").
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].pos < hits[j-1].pos; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
	var out []Ref
	index := map[string]int{}
	for _, h := range hits {
		key := strings.ToLower(h.ref.Owner + "/" + h.ref.Repo + "#" + strconv.Itoa(h.ref.Number))
		if i, seen := index[key]; seen {
			out[i].Closes = out[i].Closes || h.ref.Closes
			if out[i].Kind == "" {
				out[i].Kind = h.ref.Kind
			}
			continue
		}
		index[key] = len(out)
		out = append(out, h.ref)
	}
	return out
}

func number(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	return n, err == nil && n > 0 && n <= maxRefNumber
}

func itoa(n int) string { return strconv.Itoa(n) }
