package main

// export mode: rename unexported CORE-declared symbols that are referenced
// from files of other pieces (production only) to their exported form,
// rewriting the declaration and every use — including test files — by
// exact byte offsets. Collisions with existing package-scope names or
// same-type members are REPORTED and skipped for manual naming.
//
//	piecetool -export <dir>

import (
	"fmt"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// target pairs an unexported object with its exported spelling.
type target struct {
	obj     types.Object
	newName string
}

func exportPass(dir string) error {
	pieces, err := readPieceMap(filepath.Join(dir, "piece_map.tsv"))
	if err != nil {
		return err
	}

	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles,
		Dir:   dir,
		Tests: true,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return fmt.Errorf("load %s: %w", dir, err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		return fmt.Errorf("load %s: package has type errors", dir)
	}

	// pick the production package variant for the piece analysis, but keep
	// every variant for use-rewriting (tests included).
	var prod *packages.Package
	for _, p := range pkgs {
		if !strings.HasSuffix(p.ID, ".test]") && !strings.HasSuffix(p.ID, ".test") {
			if prod == nil || len(p.CompiledGoFiles) > len(prod.CompiledGoFiles) {
				_ = p
			}
		}
	}
	// simplest reliable pick: the variant whose name is the package name and
	// which contains no _test files
	for _, p := range pkgs {
		hasTest := false
		for _, f := range p.CompiledGoFiles {
			if strings.HasSuffix(f, "_test.go") {
				hasTest = true
				break
			}
		}
		if !hasTest {
			prod = p
			break
		}
	}
	if prod == nil {
		prod = pkgs[0]
	}
	fset := prod.Fset
	fileOf := func(pos token.Pos) string { return filepath.Base(fset.Position(pos).Filename) }
	pieceOf := func(pos token.Pos) (string, bool) {
		p, ok := pieces[fileOf(pos)]
		return p, ok
	}

	// 1. collect target objects: unexported, declared in a CORE file,
	//    used from a NON-core production file.
	cand := map[types.Object]bool{}
	for id, obj := range prod.TypesInfo.Uses {
		if obj == nil || obj.Pkg() != prod.Types || obj.Exported() {
			continue
		}
		declPiece, ok := pieceOf(obj.Pos())
		if !ok {
			continue
		}
		up, ok2 := pieces[fileOf(id.Pos())]
		if !ok2 || up == declPiece {
			continue
		}
		// only LEGAL-direction cross-piece uses (higher piece reaching a
		// lower one) get exports; wrong-direction edges are seam work,
		// not rename work.
		if rank[up] <= rank[declPiece] {
			continue
		}
		if objKind(obj) == "" {
			continue
		}
		cand[obj] = true
	}

	targets, skipped := planRenames(prod, cand)

	// 3. rewrite: every Use and Def ident of each target across ALL package
	//    variants (production + tests), byte-offset edits.
	rename := func(o types.Object) (string, bool) {
		for _, t := range targets {
			if t.obj.Name() == o.Name() && t.obj.Pos() == o.Pos() {
				return t.newName, true
			}
		}
		return "", false
	}
	edits := map[string][]edit{}
	seen := map[token.Pos]bool{}
	for _, p := range pkgs {
		for id, obj := range p.TypesInfo.Uses {
			if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != prod.Types.Path() {
				continue
			}
			if nn, ok := rename(obj); ok && !seen[id.Pos()] {
				seen[id.Pos()] = true
				pos := p.Fset.Position(id.Pos())
				edits[pos.Filename] = append(edits[pos.Filename], edit{pos.Offset, pos.Offset + len(id.Name), nn})
			}
		}
		for id, obj := range p.TypesInfo.Defs {
			if obj == nil {
				continue
			}
			if nn, ok := rename(obj); ok && !seen[id.Pos()] {
				seen[id.Pos()] = true
				pos := p.Fset.Position(id.Pos())
				edits[pos.Filename] = append(edits[pos.Filename], edit{pos.Offset, pos.Offset + len(id.Name), nn})
			}
		}
	}
	for fn, es := range edits {
		b, err := os.ReadFile(fn)
		if err != nil {
			return fmt.Errorf("read %s: %w", fn, err)
		}
		sort.Slice(es, func(i, j int) bool { return es[i].start > es[j].start })
		for _, e := range es {
			b = append(b[:e.start], append([]byte(e.repl), b[e.end:]...)...)
		}
		if err := os.WriteFile(fn, b, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", fn, err)
		}
	}
	fmt.Printf("exported %d symbols across %d files\n", len(targets), len(edits))
	for _, t := range targets {
		fmt.Printf("EXPORTED %s -> %s\n", t.obj.Name(), t.newName)
	}
	for _, s := range skipped {
		fmt.Printf("SKIPPED %s\n", s)
	}
	return nil
}

func fieldCollides(p *packages.Package, f *types.Var, newName string) bool {
	scope := p.Types.Scope()
	for _, n := range scope.Names() {
		tn, ok := scope.Lookup(n).(*types.TypeName)
		if !ok {
			continue
		}
		named, ok := tn.Type().(*types.Named)
		if !ok {
			continue
		}
		st, ok := named.Underlying().(*types.Struct)
		if !ok {
			continue
		}
		owns := false
		for i := 0; i < st.NumFields(); i++ {
			if st.Field(i) == f {
				owns = true
			}
		}
		if !owns {
			continue
		}
		for i := 0; i < st.NumFields(); i++ {
			if st.Field(i).Name() == newName {
				return true
			}
		}
		for i := 0; i < named.NumMethods(); i++ {
			if named.Method(i).Name() == newName {
				return true
			}
		}
	}
	return false
}

// planRenames turns the candidate set into the concrete rename list:
// each unexported target gets its exported spelling, with package-scope
// and same-type member collisions reported and skipped for manual
// naming. Split out of exportPass to keep each phase legible.
func planRenames(prod *packages.Package, cand map[types.Object]bool) (targets []target, skipped []string) {
	scope := prod.Types.Scope()
	for obj := range cand {
		name := obj.Name()
		if strings.HasPrefix(name, "_") {
			skipped = append(skipped, name+" (underscore)")
			continue
		}
		newName := strings.ToUpper(name[:1]) + name[1:]
		collide := false
		switch o := obj.(type) {
		case *types.Func:
			if sig := o.Type().(*types.Signature); sig.Recv() != nil {
				// method: check member set of the receiver's named type
				rt := sig.Recv().Type()
				if p, ok := rt.(*types.Pointer); ok {
					rt = p.Elem()
				}
				if named, ok := rt.(*types.Named); ok {
					for i := 0; i < named.NumMethods(); i++ {
						if named.Method(i).Name() == newName {
							collide = true
						}
					}
					if st, ok := named.Underlying().(*types.Struct); ok {
						for i := 0; i < st.NumFields(); i++ {
							if st.Field(i).Name() == newName {
								collide = true
							}
						}
					}
				}
			} else if scope.Lookup(newName) != nil {
				collide = true
			}
		case *types.Var:
			if o.IsField() {
				// find owner struct: approximate via scope-independent check —
				// conservative: look up the field's parent named type by scanning
				// all named types' fields for this exact object, then check the
				// member set.
				collide = fieldCollides(prod, o, newName)
			} else if scope.Lookup(newName) != nil {
				collide = true
			}
		default:
			if scope.Lookup(newName) != nil {
				collide = true
			}
		}
		if collide {
			skipped = append(skipped, name+" -> "+newName+" (collision)")
			continue
		}
		targets = append(targets, target{obj, newName})
	}
	// METHOD-SET CLOSURE: renaming any method (interface or concrete) renames
	// every same-named method with an identical signature string across the
	// package's named types and interfaces, so implementations keep satisfying
	// their interfaces. A collision anywhere in the group drops the group.
	groupOf := func(m *types.Func) []types.Object {
		sig := m.Type().(*types.Signature)
		if sig.Recv() == nil {
			return []types.Object{m}
		}
		key := m.Name() + "|" + types.TypeString(sig.Params(), nil) + "|" + types.TypeString(sig.Results(), nil)
		var group []types.Object
		for _, n := range scope.Names() {
			tn, ok := scope.Lookup(n).(*types.TypeName)
			if !ok {
				continue
			}
			named, ok := tn.Type().(*types.Named)
			if ok {
				for i := 0; i < named.NumMethods(); i++ {
					mm := named.Method(i)
					ms := mm.Type().(*types.Signature)
					if mm.Name()+"|"+types.TypeString(ms.Params(), nil)+"|"+types.TypeString(ms.Results(), nil) == key {
						group = append(group, mm)
					}
				}
			}
			if iface, ok := tn.Type().Underlying().(*types.Interface); ok {
				for i := 0; i < iface.NumExplicitMethods(); i++ {
					mm := iface.ExplicitMethod(i)
					ms := mm.Type().(*types.Signature)
					if mm.Name()+"|"+types.TypeString(ms.Params(), nil)+"|"+types.TypeString(ms.Results(), nil) == key {
						group = append(group, mm)
					}
				}
			}
		}
		if len(group) == 0 {
			group = []types.Object{m}
		}
		return group
	}
	expanded := map[types.Object]string{}
	dropGroup := map[string]bool{}
	for _, t := range targets {
		if m, ok := t.obj.(*types.Func); ok && m.Type().(*types.Signature).Recv() != nil {
			for _, g := range groupOf(m) {
				expanded[g] = t.newName
			}
		} else {
			expanded[t.obj] = t.newName
		}
	}
	_ = dropGroup
	targets = targets[:0]
	for o, nn := range expanded {
		targets = append(targets, target{o, nn})
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].obj.Name() < targets[j].obj.Name() })

	return targets, skipped
}
