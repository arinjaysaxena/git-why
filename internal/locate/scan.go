package locate

import (
	"regexp"
	"strings"
)

// rule recognises one kind of declaration on a (masked) line.
type rule struct {
	re   *regexp.Regexp // group 1 is the declared name
	kind string
	// container: declarations nested inside get this one's name as a prefix.
	container bool
	// register: the declaration itself can be a target (an impl block cannot).
	register bool
	// member: only valid directly inside a container.
	member bool
	// expr: the declaration may be a brace-less expression such as an arrow function.
	expr bool
}

type container struct {
	name  string
	depth int
}

var keywords = map[string]bool{
	"if": true, "for": true, "while": true, "switch": true, "catch": true, "return": true, "else": true,
	"do": true, "try": true, "new": true, "throw": true, "super": true, "this": true, "synchronized": true,
	"match": true, "loop": true, "await": true, "typeof": true, "function": true, "delete": true,
}

// scanBraces finds declarations in a brace-delimited language and returns their line ranges.
func scanBraces(src string, st maskStyle, rules []rule, decorator string) []Range {
	masked := mask(src, st)
	lines := strings.Split(masked, "\n")
	var out []Range
	var stack []container
	depth := 0
	for i, line := range lines {
		for len(stack) > 0 && depth <= stack[len(stack)-1].depth {
			stack = stack[:len(stack)-1]
		}
		base := 0
		if len(stack) > 0 {
			base = stack[len(stack)-1].depth + 1
		}
		if depth == base {
			for _, r := range rules {
				if r.member && len(stack) == 0 {
					continue
				}
				m := r.re.FindStringSubmatch(line)
				if m == nil || (r.member && keywords[m[1]]) {
					continue
				}
				name := m[1]
				qual := name
				if len(stack) > 0 {
					qual = stack[len(stack)-1].name + "." + name
				}
				if r.register {
					start := i
					for start > 0 && decorator != "" && strings.HasPrefix(strings.TrimSpace(lines[start-1]), decorator) {
						start--
					}
					end := endLine(masked, lines, i, r.expr)
					out = append(out, Range{Start: start + 1, End: end + 1, Name: qual, Kind: r.kind})
				}
				if r.container {
					stack = append(stack, container{name: qual, depth: depth})
				}
				break
			}
		}
		depth += strings.Count(line, "{") - strings.Count(line, "}")
		if depth < 0 {
			depth = 0
		}
	}
	return out
}

// endLine returns the 0-based last line of the declaration starting at line
// start: the line of the matching '}' of its body, or the line of the ';' that
// ends a body-less declaration.
func endLine(masked string, lines []string, start int, expr bool) int {
	off := 0
	for i := 0; i < start; i++ {
		off += len(lines[i]) + 1
	}
	paren, line := 0, start
	lastSig := byte(0)
	sawArrow := false
	for i := off; i < len(masked); i++ {
		c := masked[i]
		switch c {
		case '\n':
			if expr && paren == 0 && sawArrow && !strings.ContainsRune("=>(,+-*/&|?:<", rune(lastSig)) {
				return line
			}
			line++
		case '(', '[':
			paren++
		case ')', ']':
			paren--
		case ';':
			if paren <= 0 {
				return line
			}
		case '{':
			if paren > 0 {
				continue
			}
			d := 0
			for j := i; j < len(masked); j++ {
				switch masked[j] {
				case '\n':
					line++
				case '{':
					d++
				case '}':
					d--
					if d == 0 {
						return line
					}
				}
			}
			return len(lines) - 1
		case '>':
			if i > 0 && masked[i-1] == '=' {
				sawArrow = true
			}
		}
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			lastSig = c
		}
	}
	return len(lines) - 1
}

// mergeOverloads collapses same-named declarations of one kind into the span covering all of them.
func mergeOverloads(rs []Range) []Range {
	var out []Range
	idx := map[string]int{}
	for _, r := range rs {
		key := r.Kind + " " + r.Name
		if j, ok := idx[key]; ok {
			if r.Start < out[j].Start {
				out[j].Start = r.Start
			}
			if r.End > out[j].End {
				out[j].End = r.End
			}
			continue
		}
		idx[key] = len(out)
		out = append(out, r)
	}
	return out
}

// scanPython finds def and class declarations using indentation.
func scanPython(src string) []Range {
	masked := mask(src, maskStyle{lineComment: "#", singleQuote: true, triple: true})
	lines := strings.Split(masked, "\n")
	type frame struct {
		name   string
		indent int
		isDef  bool
	}
	var stack []frame
	var out []Range
	for i, line := range lines {
		m := pyDecl.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		indent := len(m[1])
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		kind, name := m[2], m[3]
		if len(stack) > 0 && stack[len(stack)-1].isDef {
			continue // declarations nested in a function body are not addressable
		}
		qual := name
		if len(stack) > 0 {
			qual = stack[len(stack)-1].name + "." + name
		}
		stack = append(stack, frame{name: qual, indent: indent, isDef: kind != "class"})
		start := i
		for start > 0 && strings.HasPrefix(strings.TrimSpace(lines[start-1]), "@") && indentOf(lines[start-1]) == indent {
			start--
		}
		out = append(out, Range{Start: start + 1, End: pyEnd(lines, i, indent) + 1, Name: qual, Kind: map[string]string{"class": "class", "def": "func"}[kind]})
	}
	for i := range out {
		if out[i].Kind == "func" && strings.Contains(out[i].Name, ".") {
			out[i].Kind = "method"
		}
	}
	return out
}

var pyDecl = regexp.MustCompile(`^(\s*)(?:async\s+)?(def|class)\s+([A-Za-z_]\w*)`)

func indentOf(line string) int { return len(line) - len(strings.TrimLeft(line, " \t")) }

// pyEnd is the last line of the block whose header starts at line start.
func pyEnd(lines []string, start, indent int) int {
	// The header may span several lines while brackets are open.
	i, open := start, 0
	for ; i < len(lines); i++ {
		open += strings.Count(lines[i], "(") + strings.Count(lines[i], "[") - strings.Count(lines[i], ")") - strings.Count(lines[i], "]")
		if open <= 0 {
			break
		}
	}
	end := i
	for j := i + 1; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == "" {
			continue
		}
		if indentOf(lines[j]) <= indent {
			break
		}
		end = j
	}
	return end
}
