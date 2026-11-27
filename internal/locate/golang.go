package locate

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

// locateGo finds functions, methods (Type.Method) and types in a Go file.
func locateGo(filePath, src, symbol string) (Range, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, src, parser.SkipObjectResolution)
	if f == nil {
		return Range{}, fmt.Errorf("locate: parse %s: %w", filePath, err)
	}
	// A syntax error elsewhere in the file must not hide a declaration that parsed.
	line := func(p token.Pos) int { return fset.Position(p).Line }
	var decls []Range
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			r := Range{Start: line(d.Pos()), End: line(d.End()), Name: d.Name.Name, Kind: "func"}
			if d.Recv != nil && len(d.Recv.List) == 1 {
				r.Name = receiverName(d.Recv.List[0].Type) + "." + d.Name.Name
				r.Kind = "method"
			}
			decls = append(decls, r)
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, s := range d.Specs {
				ts := s.(*ast.TypeSpec)
				start, end := ts.Pos(), ts.End()
				if !d.Lparen.IsValid() {
					start, end = d.Pos(), d.End() // `type T ...` without a group: include the keyword
				}
				decls = append(decls, Range{Start: line(start), End: line(end), Name: ts.Name.Name, Kind: "type"})
			}
		}
	}
	return pick(symbol, decls)
}

// receiverName extracts T from T, *T, T[P] and *T[P, Q].
func receiverName(e ast.Expr) string {
	for {
		switch t := e.(type) {
		case *ast.StarExpr:
			e = t.X
		case *ast.ParenExpr:
			e = t.X
		case *ast.IndexExpr:
			e = t.X
		case *ast.IndexListExpr:
			e = t.X
		case *ast.Ident:
			return t.Name
		default:
			return "?"
		}
	}
}
