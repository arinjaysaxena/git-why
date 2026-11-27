package locate

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

func rx(s string) *regexp.Regexp { return regexp.MustCompile(s) }

var (
	jsStyle   = maskStyle{lineComment: "//", blockComment: true, backtick: true, singleQuote: true}
	javaStyle = maskStyle{lineComment: "//", blockComment: true, triple: true}
	rustStyle = maskStyle{lineComment: "//", blockComment: true, nestedBlock: true, rawHash: true}

	tsMods = `(?:(?:public|private|protected|static|async|readonly|override|abstract|declare|get|set)\s+)*`

	tsRules = []rule{
		{re: rx(`^\s*(?:export\s+)?(?:default\s+)?(?:declare\s+)?(?:abstract\s+)?class\s+([A-Za-z_$][\w$]*)`), kind: "class", container: true, register: true},
		{re: rx(`^\s*(?:export\s+)?(?:declare\s+)?interface\s+([A-Za-z_$][\w$]*)`), kind: "interface", container: true, register: true},
		{re: rx(`^\s*(?:export\s+)?(?:declare\s+)?(?:const\s+)?enum\s+([A-Za-z_$][\w$]*)`), kind: "enum", register: true},
		{re: rx(`^\s*(?:export\s+)?(?:declare\s+)?type\s+([A-Za-z_$][\w$]*)`), kind: "type", register: true},
		{re: rx(`^\s*(?:export\s+)?(?:default\s+)?(?:declare\s+)?(?:async\s+)?function\s*\*?\s*([A-Za-z_$][\w$]*)`), kind: "func", register: true},
		{re: rx(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*(?::[^=]+)?=\s*(?:async\s*)?(?:function\b|\([^)]*\)?\s*(?::[^=]+)?(?:=>|$)|[A-Za-z_$][\w$]*\s*=>)`), kind: "func", register: true, expr: true},
		{re: rx(`^\s*` + tsMods + `([A-Za-z_$#][\w$]*)\s*(?:<[^()]*>)?\s*(?:\(|[?!]?\s*(?::[^=]+)?=\s*(?:async\s*)?(?:\(|function\b|[A-Za-z_$][\w$]*\s*=>))`), kind: "method", register: true, member: true, expr: true},
	}

	javaMods  = `(?:(?:public|private|protected|static|final|abstract|synchronized|native|default|strictfp)\s+)*`
	javaRules = []rule{
		{re: rx(`^\s*` + javaMods + `(?:class|interface|enum|record|@interface)\s+([A-Za-z_$][\w$]*)`), kind: "class", container: true, register: true},
		{re: rx(`^\s*` + javaMods + `(?:<[^>]+>\s+)?[\w.$<>\[\],? ]+?\s+([A-Za-z_$][\w$]*)\s*\(`), kind: "method", register: true, member: true},
		{re: rx(`^\s*` + javaMods + `([A-Z][\w$]*)\s*\(`), kind: "method", register: true, member: true},
	}

	rustVis   = `(?:pub(?:\([^)]*\))?\s+)?`
	rustRules = []rule{
		{re: rx(`^\s*` + rustVis + `(?:default\s+)?(?:const\s+)?(?:async\s+)?(?:unsafe\s+)?(?:extern\s+"[^"]*"\s+)?fn\s+([A-Za-z_]\w*)`), kind: "func", register: true},
		{re: rx(`^\s*` + rustVis + `(?:unsafe\s+)?impl\b(?:\s*<[^{]*?>)?\s+(?:[^{]*?\bfor\s+)?&?(?:\w+::)*([A-Za-z_]\w*)`), kind: "impl", container: true},
		{re: rx(`^\s*` + rustVis + `(?:unsafe\s+)?trait\s+([A-Za-z_]\w*)`), kind: "trait", container: true, register: true},
		{re: rx(`^\s*` + rustVis + `mod\s+([A-Za-z_]\w*)`), kind: "mod", container: true, register: true},
		{re: rx(`^\s*` + rustVis + `(?:struct|enum|union|type)\s+([A-Za-z_]\w*)`), kind: "type", register: true},
	}
)

// locateScanned handles every non-Go language.
func locateScanned(filePath, src, symbol string) (Range, error) {
	var decls []Range
	switch strings.ToLower(path.Ext(filePath)) {
	case ".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs":
		decls = mergeOverloads(scanBraces(src, jsStyle, tsRules, "@"))
	case ".java":
		decls = mergeOverloads(scanBraces(src, javaStyle, javaRules, "@"))
	case ".rs":
		decls = scanBraces(src, rustStyle, rustRules, "#[")
	case ".py", ".pyi":
		decls = scanPython(src)
	default:
		return Range{}, fmt.Errorf("%w: %s", ErrUnsupported, path.Ext(filePath))
	}
	return pick(symbol, decls)
}
