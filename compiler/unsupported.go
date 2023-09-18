package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

// unsupported checks a function for unsupported language features.
func unsupported(decl *ast.FuncDecl, info *types.Info) (err error) {
	ast.Inspect(decl, func(node ast.Node) bool {
		switch nn := node.(type) {
		case ast.Expr:
			switch nn.(type) {
			case *ast.FuncLit:
				err = fmt.Errorf("not implemented: func literals")
			}
			if countFunctionCalls(nn, info) > 1 {
				err = fmt.Errorf("not implemented: multiple function calls in an expression")
			}

		case ast.Stmt:
			switch n := nn.(type) {
			// Not yet supported:
			case *ast.DeferStmt:
				err = fmt.Errorf("not implemented: defer")
			case *ast.GoStmt:
				err = fmt.Errorf("not implemented: go")
			case *ast.SelectStmt:
				err = fmt.Errorf("not implemented: select")
			case *ast.CommClause:
				err = fmt.Errorf("not implemented: select case")

			// Partially supported:
			case *ast.BranchStmt:
				// continue/break are supported, goto/fallthrough are not.
				if n.Tok == token.GOTO {
					err = fmt.Errorf("not implemented: goto")
				} else if n.Tok == token.FALLTHROUGH {
					err = fmt.Errorf("not implemented: fallthrough")
				}
			case *ast.LabeledStmt:
				// labeled for/switch/select statements are supported,
				// arbitrary labels are not.
				switch n.Stmt.(type) {
				case *ast.ForStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
				default:
					err = fmt.Errorf("not implemented: labels not attached to for/switch/select")
				}
			case *ast.ForStmt:
				// Only very simple for loop post iteration statements
				// are supported.
				switch p := n.Post.(type) {
				case nil:
				case *ast.IncDecStmt:
					if _, ok := p.X.(*ast.Ident); !ok {
						err = fmt.Errorf("not implemented: for post inc/dec %T", p.X)
					}
				default:
					err = fmt.Errorf("not implemented: for post %T", p)
				}

			// Fully supported:
			case *ast.AssignStmt:
			case *ast.BlockStmt:
			case *ast.CaseClause:
			case *ast.DeclStmt:
			case *ast.EmptyStmt:
			case *ast.ExprStmt:
			case *ast.IfStmt:
			case *ast.IncDecStmt:
			case *ast.RangeStmt:
			case *ast.ReturnStmt:
			case *ast.SendStmt:
			case *ast.SwitchStmt:
			case *ast.TypeSwitchStmt:

			// Catch all in case new statements are added:
			default:
				err = fmt.Errorf("not implmemented: ast.Stmt(%T)", n)
			}
		}
		return err == nil
	})
	return
}

func countFunctionCalls(expr ast.Expr, info *types.Info) (count int) {
	ast.Inspect(expr, func(node ast.Node) bool {
		c, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch f := c.Fun.(type) {
		case *ast.Ident:
			if obj := info.ObjectOf(f); types.Universe.Lookup(f.Name) == obj {
				return true // skip builtins
			} else if _, ok := obj.(*types.TypeName); ok {
				return true // skip type casts
			}
		case *ast.SelectorExpr:
			if x, ok := f.X.(*ast.Ident); ok {
				if obj := info.ObjectOf(x); obj != nil {
					if pkg, ok := obj.(*types.PkgName); ok {
						pkgPath := pkg.Imported().Path()
						switch {
						case pkgPath == "unsafe":
							return true // skip unsafe intrinsics
						}
					}
				}
			}
		}
		count++
		return true
	})
	return
}