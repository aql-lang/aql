// piecetool: type-checked cross-piece inventory for the eng four-piece split.
//
// Reads eng/go/piece_map.tsv, type-checks the eng package, and reports the
// entanglements the physical cut must resolve:
//
//   - F8: unexported top-level identifiers referenced from a file of a
//     DIFFERENT piece (the export/shim work list), grouped by boundary.
//   - FOREIGN METHODS: methods declared in piece P on a named type whose
//     declaration file belongs to piece Q != P.
//   - F4 FIELDS: struct types in piece P with a field whose named type is
//     declared in a HIGHER piece (wrong-direction declaration coupling).
//
// Direction legality (dep chain eng -> compiler -> check -> core): a
// reference from a higher piece to a lower one is legal; only lower->higher
// references are reported for exported symbols. Unexported symbols are
// reported for EVERY cross-piece edge (any direction needs an export at the
// package cut).
package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

var rank = map[string]int{"core": 0, "check": 1, "compiler": 2, "eng": 3}

func main() {
	if os.Args[1] == "-demethod" {
		demethod(os.Args[2], os.Args[3:])
		return
	}
	if os.Args[1] == "-export" {
		exportPass(os.Args[2])
		return
	}
	if os.Args[1] == "-facade" {
		qual := "core"
		if len(os.Args) > 4 {
			qual = os.Args[4]
		}
		facadeFor(os.Args[2], os.Args[3], qual)
		return
	}
	if os.Args[1] == "-exports" {
		exportsMode(os.Args[2])
		return
	}
	if os.Args[1] == "-qualify" {
		qualify(os.Args[2], os.Args[3])
		return
	}
	if os.Args[1] == "-qualify-tests" {
		qualifyTests(os.Args[2], os.Args[3], os.Args[4])
		return
	}
	dir := os.Args[1] // eng/go directory
	pieces := readPieceMap(filepath.Join(dir, "piece_map.tsv"))

	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles,
		Dir:   dir,
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		panic(err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		os.Exit(1)
	}
	pkg := pkgs[0]
	fset := pkg.Fset

	fileOf := func(pos token.Pos) string { return filepath.Base(fset.Position(pos).Filename) }
	pieceOf := func(pos token.Pos) (string, bool) {
		p, ok := pieces[fileOf(pos)]
		return p, ok
	}

	// declPiece: where each top-level object lives.
	type edge struct{ from, to, ident, kind string }
	var f8 []edge
	seen := map[string]bool{}

	for id, obj := range pkg.TypesInfo.Uses {
		if obj == nil || obj.Pkg() != pkg.Types {
			continue
		}
		// top-level or method/field objects only; skip locals
		declPiece, ok := pieceOf(obj.Pos())
		if !ok {
			continue
		}
		usePiece, ok := pieceOf(id.Pos())
		if !ok || usePiece == declPiece {
			continue
		}
		kind := objKind(obj)
		if kind == "" {
			continue
		}
		if !obj.Exported() {
			key := usePiece + "->" + declPiece + ":" + kind + ":" + obj.Name()
			if !seen[key] {
				seen[key] = true
				f8 = append(f8, edge{usePiece, declPiece, obj.Name(), kind})
			}
		} else if rank[usePiece] < rank[declPiece] {
			key := "X:" + usePiece + "->" + declPiece + ":" + kind + ":" + obj.Name()
			if !seen[key] {
				seen[key] = true
				f8 = append(f8, edge{usePiece, declPiece, obj.Name(), "WRONGDIR-" + kind})
			}
		}
	}

	// foreign methods: method decl in piece P, receiver named type declared in Q != P
	type fm struct{ methPiece, typePiece, recv, name string }
	var foreign []fm
	for _, f := range pkg.Syntax {
		fp, ok := pieceOf(f.Package)
		if !ok {
			continue
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
				continue
			}
			t := fd.Recv.List[0].Type
			if st, ok := t.(*ast.StarExpr); ok {
				t = st.X
			}
			idt, ok := t.(*ast.Ident)
			if !ok {
				continue
			}
			obj := pkg.TypesInfo.Uses[idt]
			if obj == nil {
				obj = pkg.TypesInfo.Defs[idt]
			}
			if obj == nil {
				continue
			}
			tp, ok := pieceOf(obj.Pos())
			if !ok || tp == fp {
				continue
			}
			foreign = append(foreign, fm{fp, tp, idt.Name, fd.Name.Name})
		}
	}

	// F4: struct fields in piece P typed by a named type declared in piece Q with rank(Q) > rank(P)
	type f4e struct{ owner, ownerPiece, field, ftype, ftypePiece string }
	var f4 []f4e
	for _, f := range pkg.Syntax {
		fp, ok := pieceOf(f.Package)
		if !ok {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}
			for _, fld := range st.Fields.List {
				ast.Inspect(fld.Type, func(tn ast.Node) bool {
					ti, ok := tn.(*ast.Ident)
					if !ok {
						return true
					}
					obj := pkg.TypesInfo.Uses[ti]
					if obj == nil || obj.Pkg() != pkg.Types {
						return true
					}
					if _, isType := obj.(*types.TypeName); !isType {
						return true
					}
					tp, ok := pieceOf(obj.Pos())
					if !ok || rank[tp] <= rank[fp] {
						return true
					}
					names := "_"
					if len(fld.Names) > 0 {
						var ns []string
						for _, nm := range fld.Names {
							ns = append(ns, nm.Name)
						}
						names = strings.Join(ns, ",")
					}
					f4 = append(f4, f4e{ts.Name.Name, fp, names, ti.Name, tp})
					return true
				})
			}
			return true
		})
	}

	// report
	sort.Slice(f8, func(i, j int) bool {
		a, b := f8[i], f8[j]
		if a.from != b.from {
			return a.from < b.from
		}
		if a.to != b.to {
			return a.to < b.to
		}
		return a.ident < b.ident
	})
	counts := map[string]int{}
	for _, e := range f8 {
		counts[e.from+"->"+e.to]++
	}
	fmt.Println("== F8 unexported cross-piece references (+ WRONGDIR exported) ==")
	var keys []string
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%-20s %d\n", k, counts[k])
	}
	fmt.Println()
	for _, e := range f8 {
		fmt.Printf("F8 %s->%s %s %s\n", e.from, e.to, e.kind, e.ident)
	}
	fmt.Println()
	fmt.Println("== FOREIGN METHODS (decl piece != receiver type's piece) ==")
	sort.Slice(foreign, func(i, j int) bool {
		a, b := foreign[i], foreign[j]
		if a.methPiece != b.methPiece {
			return a.methPiece < b.methPiece
		}
		if a.recv != b.recv {
			return a.recv < b.recv
		}
		return a.name < b.name
	})
	for _, m := range foreign {
		fmt.Printf("FM %s-file method on %s.%s: (%s).%s\n", m.methPiece, m.typePiece, m.recv, m.recv, m.name)
	}
	fmt.Println()
	fmt.Println("== F4 wrong-direction struct fields ==")
	for _, e := range f4 {
		fmt.Printf("F4 %s[%s].%s : %s[%s]\n", e.owner, e.ownerPiece, e.field, e.ftype, e.ftypePiece)
	}
}

func objKind(obj types.Object) string {
	switch o := obj.(type) {
	case *types.Func:
		if o.Type().(*types.Signature).Recv() != nil {
			return "meth"
		}
		return "func"
	case *types.TypeName:
		return "type"
	case *types.Var:
		if o.IsField() {
			return "field"
		}
		if o.Parent() == o.Pkg().Scope() {
			return "var"
		}
		return ""
	case *types.Const:
		if o.Parent() == o.Pkg().Scope() {
			return "const"
		}
		return ""
	}
	return ""
}

func readPieceMap(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	m := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) >= 2 {
			m[parts[0]] = parts[1]
		}
	}
	return m
}
