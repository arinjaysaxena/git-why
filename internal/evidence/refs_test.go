package evidence

import (
	"reflect"
	"testing"
)

func TestParseRefs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Ref
	}{
		{"none", "plain text, color #fff and #abc123", nil},
		{"short", "see #12 and GH-34", []Ref{{Number: 12, Raw: "#12"}, {Number: 34, Raw: "GH-34"}}},
		{"lowercase gh", "relates to gh-7.", []Ref{{Number: 7, Raw: "gh-7"}}},
		{"start of text and punctuation", "#5, (#6) [#7]", []Ref{{Number: 5, Raw: "#5"}, {Number: 6, Raw: "#6"}, {Number: 7, Raw: "#7"}}},
		{"closing keywords", "Fixes #799\ncloses: #800, resolved #801, see #802", []Ref{
			{Number: 799, Closes: true, Raw: "#799"}, {Number: 800, Closes: true, Raw: "#800"},
			{Number: 801, Closes: true, Raw: "#801"}, {Number: 802, Raw: "#802"}}},
		{"duplicates merge and keep closes", "see #9 ... fixes #9", []Ref{{Number: 9, Closes: true, Raw: "#9"}}},
		{"issue url", "https://github.com/acme/app/issues/640#issuecomment-1", []Ref{
			{Number: 640, Owner: "acme", Repo: "app", Kind: "issue", Raw: "https://github.com/acme/app/issues/640#issuecomment-1"}}},
		{"pull url on enterprise host", "Closes https://ghe.example.com:8443/acme/app/pull/12", []Ref{
			{Number: 12, Owner: "acme", Repo: "app", Kind: "pr", Closes: true, Raw: "https://ghe.example.com:8443/acme/app/pull/12"}}},
		{"cross repo", "blocked by other-org/lib.go#31", []Ref{{Number: 31, Owner: "other-org", Repo: "lib.go", Raw: "other-org/lib.go#31"}}},
		{"url then short ref", "https://github.com/a/b/pull/3 and #4", []Ref{
			{Number: 3, Owner: "a", Repo: "b", Kind: "pr", Raw: "https://github.com/a/b/pull/3"}, {Number: 4, Raw: "#4"}}},
		{"not refs", "foo#12 &#39; abc#9 x/#1 #0 #-3", nil},
		{"huge numbers ignored", "#123456789012", nil},
		{"order follows text", "#2 then owner/r#1 then #3", []Ref{
			{Number: 2, Raw: "#2"}, {Number: 1, Owner: "owner", Repo: "r", Raw: "owner/r#1"}, {Number: 3, Raw: "#3"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseRefs(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseRefs(%q)\n got  %+v\n want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestRefLocal(t *testing.T) {
	if !(Ref{Number: 1}).Local("a", "b") {
		t.Error("bare ref must be local")
	}
	if !(Ref{Owner: "A", Repo: "B"}).Local("a", "b") {
		t.Error("match must ignore case")
	}
	if (Ref{Owner: "x", Repo: "b"}).Local("a", "b") {
		t.Error("other owner must not be local")
	}
}
