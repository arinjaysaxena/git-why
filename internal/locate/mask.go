package locate

import "strings"

// maskStyle describes the comment and string syntax of a language.
type maskStyle struct {
	lineComment  string // "//" or "#"
	blockComment bool   // /* ... */
	nestedBlock  bool   // Rust nests block comments
	backtick     bool   // JS template literals and Go raw strings
	singleQuote  bool   // 'x' starts a string (JS, Python); false for Java/Rust where it is a char or lifetime
	triple       bool   // """ and ''' strings (Python); """ text blocks (Java)
	rawHash      bool   // Rust r#"..."#
}

// mask replaces the contents of comments and string literals with spaces so
// later passes can count braces and match declarations without being fooled
// by text inside them. Newlines are kept so line numbers stay valid.
func mask(src string, st maskStyle) string {
	b := []byte(src)
	out := make([]byte, len(b))
	copy(out, b)
	blank := func(from, to int) {
		for i := from; i < to && i < len(out); i++ {
			if out[i] != '\n' {
				out[i] = ' '
			}
		}
	}
	n := len(b)
	for i := 0; i < n; {
		c := b[i]
		switch {
		case st.lineComment != "" && strings.HasPrefix(src[i:], st.lineComment):
			j := i
			for j < n && b[j] != '\n' {
				j++
			}
			blank(i, j)
			i = j
		case st.blockComment && strings.HasPrefix(src[i:], "/*"):
			j, depth := i+2, 1
			for j < n && depth > 0 {
				switch {
				case strings.HasPrefix(src[j:], "*/"):
					depth--
					j += 2
				case st.nestedBlock && strings.HasPrefix(src[j:], "/*"):
					depth++
					j += 2
				default:
					j++
				}
			}
			blank(i, j)
			i = j
		case st.rawHash && c == 'r' && (i == 0 || !isIdent(b[i-1])) && rawStart(src[i:]) > 0:
			hashes := rawStart(src[i:])
			end := "\"" + strings.Repeat("#", hashes-1)
			j := i + hashes + 1
			k := strings.Index(src[j:], end)
			if k < 0 {
				k = n - j
			}
			blank(j, j+k)
			i = j + k + len(end)
		case st.triple && (strings.HasPrefix(src[i:], `"""`) || (st.singleQuote && strings.HasPrefix(src[i:], `'''`))):
			q := src[i : i+3]
			k := strings.Index(src[i+3:], q)
			end := n
			if k >= 0 {
				end = i + 3 + k
			}
			blank(i+3, end)
			i = end + 3
		case c == '"' || (c == '\'' && st.singleQuote) || (c == '`' && st.backtick):
			j := i + 1
			for j < n && b[j] != c && (b[j] != '\n' || c == '`') {
				if b[j] == '\\' && c != '`' {
					j++
				}
				j++
			}
			blank(i+1, j)
			i = j + 1
		case c == '\'':
			// Java/Rust: char literal 'x' or '\n'; otherwise a Rust lifetime, left alone.
			switch {
			case i+2 < n && b[i+1] != '\\' && b[i+2] == '\'':
				blank(i+1, i+2)
				i += 3
			case i+1 < n && b[i+1] == '\\':
				j := i + 2
				for j < n && b[j] != '\'' && b[j] != '\n' {
					j++
				}
				blank(i+1, j)
				i = j + 1
			default:
				i++
			}
		default:
			i++
		}
	}
	return string(out)
}

func isIdent(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// rawStart returns the number of '#' plus one when s starts a Rust raw string
// (r"...", r#"..."#), else 0.
func rawStart(s string) int {
	i := 1
	for i < len(s) && s[i] == '#' {
		i++
	}
	if i < len(s) && s[i] == '"' {
		return i
	}
	return 0
}
