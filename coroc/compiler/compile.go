package compiler

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

const coroutinePackage = "github.com/stealthrocket/coroutine"

// Compile compiles coroutines in one or more packages.
//
// The path argument can either be a path to a package, a
// path to a file within a package, or a pattern that matches
// multiple packages (for example, /path/to/package/...).
// The path can be absolute or relative (to the current working
// directory).
func Compile(path string, options ...CompileOption) error {
	c := &compiler{
		outputFilename: "coroc_generated.go",
		fset:           token.NewFileSet(),
	}
	for _, option := range options {
		option(c)
	}
	return c.compile(path)
}

// CompileOption configures the compiler.
type CompileOption func(*compiler)

// WithOutputFilename instructs the compiler to write generated code
// to a file with the specified name within each package that contains
// coroutines.
func WithOutputFilename(outputFilename string) CompileOption {
	return func(c *compiler) { c.outputFilename = outputFilename }
}

// WithBuildTags instructs the compiler to attach the specified build
// tags to generated files.
func WithBuildTags(buildTags string) CompileOption {
	return func(c *compiler) { c.buildTags = buildTags }
}

type compiler struct {
	outputFilename string
	buildTags      string

	fset *token.FileSet
}

func (c *compiler) compile(path string) error {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	if path != "" && !strings.HasSuffix(path, "...") {
		s, err := os.Stat(path)
		if err != nil {
			return err
		} else if !s.IsDir() {
			// Make sure we're loading whole packages.
			path = filepath.Dir(path)
		}
	}
	path = filepath.Clean(path)
	if len(path) > 0 && path[0] != filepath.Separator && path[0] != '.' {
		// Go interprets patterns without a leading dot as part of the
		// stdlib (i.e. part of $GOROOT/src) rather than relative to
		// the working dir. Note that filepath.Join(".", path) does not
		// give the desired result here, hence the manual concat.
		path = "." + string(filepath.Separator) + path
	}

	log.Printf("reading, parsing and type-checking")
	conf := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedImports | packages.NeedDeps | packages.NeedTypesInfo,
		Fset: c.fset,
	}
	pkgs, err := packages.Load(conf, path)
	if err != nil {
		return fmt.Errorf("packages.Load %q: %w", path, err)
	}
	flatpkgs := flattenPackages(pkgs)
	for _, p := range flatpkgs {
		for _, err := range p.Errors {
			return err
		}
	}

	log.Printf("building SSA program")
	prog, _ := ssautil.Packages(pkgs, ssa.InstantiateGenerics|ssa.GlobalDebug)
	prog.Build()

	log.Printf("building call graph")
	cg := cha.CallGraph(prog)

	log.Printf("finding generic yield instantiations")
	var coroutinePkg *packages.Package
	for _, p := range flatpkgs {
		if p.PkgPath == coroutinePackage {
			coroutinePkg = p
			break
		}
	}
	if coroutinePkg == nil {
		log.Printf("%s not imported by the module. Nothing to do", coroutinePackage)
		return nil
	}
	yieldFunc := prog.FuncValue(coroutinePkg.Types.Scope().Lookup("Yield").(*types.Func))
	yieldInstances := functionColors{}
	for fn := range ssautil.AllFunctions(prog) {
		if fn.Origin() == yieldFunc {
			yieldInstances[fn] = fn.Signature
		}
	}

	log.Printf("coloring functions")
	colors, err := colorFunctions(cg, yieldInstances)
	if err != nil {
		return err
	}
	pkgsByTypes := map[*types.Package]*packages.Package{}
	for _, p := range flatpkgs {
		pkgsByTypes[p.Types] = p
	}
	colorsByPkg := map[*packages.Package]functionColors{}
	for fn, color := range colors {
		if fn.Pkg == nil {
			return fmt.Errorf("unsupported yield function %s (Pkg is nil)", fn)
		}

		p := pkgsByTypes[fn.Pkg.Pkg]
		pkgColors := colorsByPkg[p]
		if pkgColors == nil {
			pkgColors = functionColors{}
			colorsByPkg[p] = pkgColors
		}
		pkgColors[fn] = color
	}

	for p, colors := range colorsByPkg {
		if err := c.compilePackage(p, colors); err != nil {
			return err
		}
	}

	log.Printf("done")

	return nil
}

func (c *compiler) compilePackage(p *packages.Package, colors functionColors) error {
	log.Printf("compiling package %s", p.Name)

	// Generate the coroutine AST.
	gen := &ast.File{
		Name: ast.NewIdent(p.Name),
	}
	gen.Decls = append(gen.Decls, &ast.GenDecl{
		Tok: token.IMPORT,
		Specs: []ast.Spec{
			&ast.ImportSpec{
				Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(coroutinePackage)},
			},
		},
	})

	colorsByDecl := map[*ast.FuncDecl]*types.Signature{}
	for fn, color := range colors {
		decl, ok := fn.Syntax().(*ast.FuncDecl)
		if !ok {
			return fmt.Errorf("unsupported yield function %s (Syntax is %T, not *ast.FuncDecl)", fn, fn.Syntax())
		}
		colorsByDecl[decl] = color
	}
	for _, f := range p.Syntax {
		for _, anydecl := range f.Decls {
			decl, ok := anydecl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			color, ok := colorsByDecl[decl]
			if !ok {
				continue
			}

			// Reject certain language features for now.
			var err error
			ast.Inspect(decl, func(node ast.Node) bool {
				stmt, ok := node.(ast.Stmt)
				if !ok {
					return true
				}
				switch n := stmt.(type) {
				// Not supported:
				case *ast.DeferStmt:
					err = fmt.Errorf("not implemented: defer")
				case *ast.GoStmt:
					err = fmt.Errorf("not implemented: go")
				case *ast.LabeledStmt:
					err = fmt.Errorf("not implemented: labels")
				case *ast.TypeSwitchStmt:
					err = fmt.Errorf("not implemented: type switch")
				case *ast.SelectStmt:
					err = fmt.Errorf("not implemented: select")
				case *ast.CommClause:
					err = fmt.Errorf("not implemented: select case")
				case *ast.DeclStmt:
					err = fmt.Errorf("not implemented: inline decls")

				// Partially supported:
				case *ast.RangeStmt:
					switch t := p.TypesInfo.TypeOf(n.X).(type) {
					case *types.Array, *types.Slice:
					default:
						err = fmt.Errorf("not implemented: for range for %T", t)
					}
				case *ast.AssignStmt:
					if len(n.Lhs) != 1 || len(n.Lhs) != len(n.Rhs) {
						err = fmt.Errorf("not implemented: multiple assign")
					}
					if _, ok := n.Lhs[0].(*ast.Ident); !ok {
						err = fmt.Errorf("not implemented: assign to non-ident")
					}
				case *ast.BranchStmt:
					if n.Tok == token.GOTO {
						err = fmt.Errorf("not implemented: goto")
					} else if n.Tok == token.FALLTHROUGH {
						err = fmt.Errorf("not implemented: fallthrough")
					} else if n.Tok == token.BREAK {
						err = fmt.Errorf("not implemented: break")
					} else if n.Tok == token.CONTINUE {
						err = fmt.Errorf("not implemented: continue")
					} else if n.Label != nil {
						err = fmt.Errorf("not implemented: labeled branch")
					}
				case *ast.ForStmt:
					// Since we aren't desugaring for loop post iteration
					// statements yet, check that it's a simple increment
					// or decrement.
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
				case *ast.BlockStmt:
				case *ast.CaseClause:
				case *ast.EmptyStmt:
				case *ast.ExprStmt:
				case *ast.IfStmt:
				case *ast.IncDecStmt:
				case *ast.ReturnStmt:
				case *ast.SendStmt:
				case *ast.SwitchStmt:

				// Catch all in case new statements are added:
				default:
					err = fmt.Errorf("not implmemented: ast.Stmt(%T)", n)
				}
				return err == nil
			})
			if err != nil {
				return err
			}

			gen.Decls = append(gen.Decls, c.compileFunction(p, decl, color))
		}
	}

	// Get ready to write.
	packageDir := filepath.Dir(p.GoFiles[0])
	outputPath := filepath.Join(packageDir, c.outputFilename)
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("os.Create %q: %w", outputPath, err)
	}
	defer outputFile.Close()

	// Comments are awkward to attach to the tree (they rely on token.Pos, which
	// is coupled to a token.FileSet). Instead, just write out the raw strings.
	var b strings.Builder
	b.WriteString(`// Code generated by coroc. DO NOT EDIT`)
	b.WriteString("\n\n")
	if c.buildTags != "" {
		b.WriteString(`//go:build `)
		b.WriteString(c.buildTags)
		b.WriteString("\n\n")
	}
	if _, err := outputFile.WriteString(b.String()); err != nil {
		return err
	}

	// Format/write the remainder of the AST.
	if err := format.Node(outputFile, c.fset, gen); err != nil {
		return err
	}
	return outputFile.Close()
}

func (c *compiler) compileFunction(p *packages.Package, fn *ast.FuncDecl, color *types.Signature) *ast.FuncDecl {
	log.Printf("compiling function %s %s", p.Name, fn.Name)

	// Generate the coroutine function. At this stage, use the same name
	// as the source function (and require that the caller use build tags
	// to disambiguate function calls).
	gen := &ast.FuncDecl{
		Name: fn.Name,
		Type: fn.Type,
		Body: &ast.BlockStmt{},
	}

	ctx := ast.NewIdent("_c")
	frame := ast.NewIdent("_f")

	yieldTypeExpr := make([]ast.Expr, 2)
	yieldTypeExpr[0] = typeExpr(color.Params().At(0).Type())
	yieldTypeExpr[1] = typeExpr(color.Results().At(0).Type())

	// _c := coroutine.LoadContext[R, S]()
	gen.Body.List = append(gen.Body.List, &ast.AssignStmt{
		Lhs: []ast.Expr{ctx},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{
			&ast.CallExpr{
				Fun: &ast.IndexListExpr{
					X: &ast.SelectorExpr{
						X:   ast.NewIdent("coroutine"),
						Sel: ast.NewIdent("LoadContext"),
					},
					Indices: yieldTypeExpr,
				},
			},
		},
	})

	// _f := _c.Push()
	gen.Body.List = append(gen.Body.List, &ast.AssignStmt{
		Lhs: []ast.Expr{frame},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{
			&ast.CallExpr{
				Fun: &ast.SelectorExpr{X: ctx, Sel: ast.NewIdent("Push")},
			},
		},
	})

	// Desugar statements in the tree.
	desugar(fn.Body, p.TypesInfo)

	// Scan/replace variables defined in the function.
	objectVars := map[*ast.Object]*ast.Ident{}
	var varNames []*ast.Ident
	var varTypes []types.Type
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.AssignStmt:
			name := n.Lhs[0].(*ast.Ident)
			if n.Tok == token.DEFINE {
				n.Tok = token.ASSIGN
			}
			if name.Obj == nil {
				return true
			}
			if _, ok := objectVars[name.Obj]; ok {
				return true
			}
			varName := ast.NewIdent("_v" + strconv.Itoa(len(varNames)))
			varTypes = append(varTypes, p.TypesInfo.TypeOf(name))
			varNames = append(varNames, varName)
			objectVars[name.Obj] = varName
		}
		return true
	})
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if ident, ok := node.(*ast.Ident); ok {
			if replacement, ok := objectVars[ident.Obj]; ok {
				ident.Name = replacement.Name
			}
		}
		return true
	})

	// Declare variables upfront.
	if len(varTypes) > 0 {
		varDecl := &ast.GenDecl{Tok: token.VAR}
		for i, t := range varTypes {
			varDecl.Specs = append(varDecl.Specs, &ast.ValueSpec{
				Names: []*ast.Ident{varNames[i]},
				Type:  typeExpr(t),
			})
		}
		gen.Body.List = append(gen.Body.List, &ast.DeclStmt{Decl: varDecl})
	}

	// Collect params/results/variables that need to be saved/restored.
	var saveAndRestoreNames []*ast.Ident
	var saveAndRestoreTypes []types.Type
	if fn.Type.Params != nil {
		for _, param := range fn.Type.Params.List {
			for _, name := range param.Names {
				if name.Name != "_" {
					saveAndRestoreNames = append(saveAndRestoreNames, name)
					saveAndRestoreTypes = append(saveAndRestoreTypes, p.TypesInfo.TypeOf(name))
				}
			}
		}
	}
	if fn.Type.Results != nil {
		// Named results could be used as scratch space at any point
		// during execution, so they need to be saved/restored.
		for _, result := range fn.Type.Results.List {
			for _, name := range result.Names {
				if name.Name != "_" {
					saveAndRestoreNames = append(saveAndRestoreNames, name)
					saveAndRestoreTypes = append(saveAndRestoreTypes, p.TypesInfo.TypeOf(name))
				}
			}
		}
	}
	saveAndRestoreNames = append(saveAndRestoreNames, varNames...)
	saveAndRestoreTypes = append(saveAndRestoreTypes, varTypes...)

	// Restore state when rewinding the stack.
	//
	// As an optimization, only those variables still in scope for a
	// particular f.IP need to be restored.
	var restoreStmts []ast.Stmt
	for i, name := range saveAndRestoreNames {
		restoreStmts = append(restoreStmts, &ast.AssignStmt{
			Lhs: []ast.Expr{name},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{
				&ast.TypeAssertExpr{
					X: &ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   frame,
							Sel: ast.NewIdent("Get"),
						},
						Args: []ast.Expr{
							&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(i)},
						},
					},
					Type: typeExpr(saveAndRestoreTypes[i]),
				},
			},
		})
	}
	gen.Body.List = append(gen.Body.List, &ast.IfStmt{
		Cond: &ast.BinaryExpr{
			X:  &ast.SelectorExpr{X: ast.NewIdent("_f"), Sel: ast.NewIdent("IP")},
			Op: token.GTR, /* > */
			Y:  &ast.BasicLit{Kind: token.INT, Value: "0"}},
		Body: &ast.BlockStmt{List: restoreStmts},
	})

	// Save state when unwinding the stack.
	var saveStmts []ast.Stmt
	for i, name := range saveAndRestoreNames {
		saveStmts = append(saveStmts, &ast.ExprStmt{
			X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{X: frame, Sel: ast.NewIdent("Set")},
				Args: []ast.Expr{
					&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(i)},
					name,
				},
			},
		})
	}
	gen.Body.List = append(gen.Body.List, &ast.DeferStmt{
		Call: &ast.CallExpr{
			Fun: &ast.FuncLit{
				Type: &ast.FuncType{},
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.IfStmt{
							Cond: &ast.CallExpr{
								Fun: &ast.SelectorExpr{X: ctx, Sel: ast.NewIdent("Unwinding")},
							},
							Body: &ast.BlockStmt{List: saveStmts},
							Else: &ast.BlockStmt{List: []ast.Stmt{
								&ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ctx, Sel: ast.NewIdent("Pop")}}}},
							},
						},
					},
				},
			},
		},
	})

	spans := trackSpans(fn.Body)

	compiledBody := c.compileStatement(fn.Body, spans).(*ast.BlockStmt)

	gen.Body.List = append(gen.Body.List, compiledBody.List...)

	return gen
}

func (c *compiler) compileStatement(stmt ast.Stmt, spans map[ast.Stmt]span) ast.Stmt {
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		switch {
		case len(s.List) == 1:
			child := c.compileStatement(s.List[0], spans)
			s.List[0] = unnestBlocks(child)
		case len(s.List) > 1:
			stmt = &ast.BlockStmt{List: []ast.Stmt{c.compileDispatch(s.List, spans)}}
		}
	case *ast.IfStmt:
		s.Body = c.compileStatement(s.Body, spans).(*ast.BlockStmt)
	case *ast.ForStmt:
		forSpan := spans[s]
		s.Body = c.compileStatement(s.Body, spans).(*ast.BlockStmt)
		s.Body.List = append(s.Body.List, &ast.AssignStmt{
			Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent("_f"), Sel: ast.NewIdent("IP")}},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(forSpan.start)}},
		})
	case *ast.SwitchStmt:
		for i, child := range s.Body.List {
			s.Body.List[i] = c.compileStatement(child, spans)
		}
	case *ast.CaseClause:
		switch {
		case len(s.Body) == 1:
			child := c.compileStatement(s.Body[0], spans)
			s.Body[0] = unnestBlocks(child)
		case len(s.Body) > 1:
			s.Body = []ast.Stmt{c.compileDispatch(s.Body, spans)}
		}
	}
	return stmt
}

func (c *compiler) compileDispatch(stmts []ast.Stmt, spans map[ast.Stmt]span) ast.Stmt {
	var cases []ast.Stmt
	for i, child := range stmts {
		childSpan := spans[child]
		compiledChild := c.compileStatement(child, spans)
		compiledChild = unnestBlocks(compiledChild)
		caseBody := []ast.Stmt{compiledChild}
		if i < len(stmts)-1 {
			caseBody = append(caseBody,
				&ast.AssignStmt{
					Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent("_f"), Sel: ast.NewIdent("IP")}},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(childSpan.end)}},
				},
				&ast.BranchStmt{Tok: token.FALLTHROUGH})
		}
		cases = append(cases, &ast.CaseClause{
			List: []ast.Expr{
				&ast.BinaryExpr{
					X:  &ast.SelectorExpr{X: ast.NewIdent("_f"), Sel: ast.NewIdent("IP")},
					Op: token.LSS, /* < */
					Y:  &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(childSpan.end)}},
			},
			Body: caseBody,
		})
	}
	return &ast.SwitchStmt{Body: &ast.BlockStmt{List: cases}}
}