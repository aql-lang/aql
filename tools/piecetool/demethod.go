package main

// demethod mode: convert methods on a receiver type into free functions
// with the receiver as first parameter, rewriting every call site
// (production and tests) by exact byte offsets from the type-checked AST.
//
//	piecetool -demethod <dir> Recv.Name [Recv.Name ...]

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

type edit struct {
	start, end int // byte offsets in file
	repl       string
}

func demethod(dir string, specs []string) {
	want := map[string]map[string]bool{} // recv type name -> method name
	for _, s := range specs {
		parts := strings.SplitN(s, ".", 2)
		if len(parts) != 2 {
			panic("bad spec " + s)
		}
		if want[parts[0]] == nil {
			want[parts[0]] = map[string]bool{}
		}
		want[parts[0]][parts[1]] = true
	}

	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles,
		Dir:   dir,
		Tests: true,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		panic(err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		os.Exit(1)
	}

	// collect target *types.Func objects (from any of the package variants)
	targets := map[*types.Func]bool{}
	byName := map[string]bool{}
	for _, pkg := range pkgs {
		scope := pkg.Types.Scope()
		for recv, methods := range want {
			obj := scope.Lookup(recv)
			tn, ok := obj.(*types.TypeName)
			if !ok {
				continue
			}
			named, ok := tn.Type().(*types.Named)
			if !ok {
				continue
			}
			for i := 0; i < named.NumMethods(); i++ {
				m := named.Method(i)
				if methods[m.Name()] {
					targets[m] = true
					byName[recv+"."+m.Name()] = true
				}
			}
		}
	}
	for _, s := range specs {
		if !byName[s] {
			fmt.Fprintf(os.Stderr, "NOT FOUND: %s\n", s)
			os.Exit(1)
		}
	}
	isTarget := func(o types.Object) bool {
		f, ok := o.(*types.Func)
		if !ok {
			return false
		}
		for t := range targets {
			if t.Name() == f.Name() && t.Pos() == f.Pos() {
				return true
			}
		}
		return false
	}

	edits := map[string][]edit{} // filename -> edits
	addEdit := func(fset *token.FileSet, start, end token.Pos, repl string) {
		p1, p2 := fset.Position(start), fset.Position(end)
		edits[p1.Filename] = append(edits[p1.Filename], edit{p1.Offset, p2.Offset, repl})
	}
	src := map[string][]byte{}
	readSrc := func(fn string) []byte {
		if b, ok := src[fn]; ok {
			return b
		}
		b, err := os.ReadFile(fn)
		if err != nil {
			panic(err)
		}
		src[fn] = b
		return b
	}

	seenFile := map[string]bool{}
	for _, pkg := range pkgs {
		fset := pkg.Fset
		for _, f := range pkg.Syntax {
			fn := fset.Position(f.Package).Filename
			if seenFile[fn] {
				continue
			}
			seenFile[fn] = true
			// 1. rewrite decls
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || fd.Recv == nil {
					continue
				}
				obj := pkg.TypesInfo.Defs[fd.Name]
				if obj == nil || !isTarget(obj) {
					continue
				}
				b := readSrc(fn)
				recvField := fd.Recv.List[0]
				p1 := fset.Position(fd.Recv.Opening).Offset
				p2 := fset.Position(fd.Recv.Closing).Offset
				recvText := string(b[fset.Position(recvField.Pos()).Offset:fset.Position(recvField.End()).Offset])
				// remove "(recv) " between func and name
				addEdit(fset, fd.Recv.Opening, fd.Name.Pos(), "")
				_ = p1
				_ = p2
				// insert recv as first param
				parOpen := fd.Type.Params.Opening
				sep := ""
				if len(fd.Type.Params.List) > 0 {
					sep = ", "
				}
				addEdit(fset, parOpen+1, parOpen+1, recvText+sep)
			}
			// 2. rewrite call sites x.Name(args) -> Name(x, args)
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				obj := pkg.TypesInfo.Uses[sel.Sel]
				if obj == nil || !isTarget(obj) {
					return true
				}
				b := readSrc(fn)
				recvText := string(b[fset.Position(sel.X.Pos()).Offset:fset.Position(sel.X.End()).Offset])
				sep := ""
				if len(call.Args) > 0 {
					sep = ", "
				}
				// replace "recv.Name(" with "Name(recv<sep>"
				addEdit(fset, sel.X.Pos(), call.Lparen+1, sel.Sel.Name+"("+recvText+sep)
				return true
			})
			// 3. method values (*Engine).Name / e.Name used as value: rewrite bare
			// (handled manually if any — report)
		}
	}

	for fn, es := range edits {
		b := readSrc(fn)
		sort.Slice(es, func(i, j int) bool { return es[i].start > es[j].start })
		for _, e := range es {
			b = append(b[:e.start], append([]byte(e.repl), b[e.end:]...)...)
		}
		if err := os.WriteFile(fn, b, 0o644); err != nil {
			panic(err)
		}
		fmt.Printf("rewrote %s (%d edits)\n", fn, len(es))
	}
}
