package main

// qualify mode: rewrite a piece's files so every use of a package-scope
// symbol DECLARED IN A DIFFERENT PIECE is qualified with that piece's
// package qualifier (core.X / check.X / compiler.X). Prepares the
// physical module cut: after qualification a piece's files compile
// verbatim in their own package with ordinary imports. Only bare
// identifier uses of package-scope funcs/types/vars/consts are
// rewritten (methods, fields, and locals resolve through values and
// need no qualifier). Rewrites are by exact byte offsets, largest
// offset first.
//
//	piecetool -qualify <dir> <piece>
//
// The facade files (piece "core" rows aliases_core*.go) are treated as
// core declarations: a use resolving to an alias qualifies as core.X
// (the alias name is the core name by construction).

import (
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

var pieceQualifier = map[string]string{
	"core": "core", "check": "check", "compiler": "compiler", "eng": "eng",
}

func qualify(dir, piece string) {
	qualifyImpl(dir, piece, nil)
}

// qualifyTests: qualify cross-piece PRODUCTION-symbol uses inside the
// named test files, as preparation for moving them to the target
// piece's module. Type errors are tolerated (the files may reference
// internals that only resolve after the move).
//
//	piecetool -qualify-tests <dir> <targetPiece> <file,file,...>
func qualifyTests(dir, target, list string) {
	only := map[string]bool{}
	for _, f := range strings.Split(list, ",") {
		only[strings.TrimSpace(f)] = true
	}
	qualifyImpl(dir, target, only)
}

func qualifyImpl(dir, piece string, onlyFiles map[string]bool) {
	pieces := readPieceMap(filepath.Join(dir, "piece_map.tsv"))

	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles,
		Dir:   dir,
		Tests: onlyFiles != nil,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		panic(err)
	}
	if onlyFiles == nil && packages.PrintErrors(pkgs) > 0 {
		os.Exit(1)
	}
	pkg := pkgs[0]
	if onlyFiles != nil {
		// pick the test variant (the package with the most files)
		for _, p := range pkgs {
			if len(p.CompiledGoFiles) > len(pkg.CompiledGoFiles) {
				pkg = p
			}
		}
	}
	fset := pkg.Fset

	fileOf := func(pos token.Pos) string { return filepath.Base(fset.Position(pos).Filename) }
	pieceOf := func(pos token.Pos) (string, bool) {
		p, ok := pieces[fileOf(pos)]
		return p, ok
	}

	// edits per absolute filename: insert qualifier before ident start.
	type edit struct {
		off int
		ins string
	}
	edits := map[string][]edit{}
	counts := map[string]int{}

	for id, obj := range pkg.TypesInfo.Uses {
		if obj == nil || obj.Pkg() != pkg.Types {
			continue
		}
		if onlyFiles != nil {
			if !onlyFiles[fileOf(id.Pos())] {
				continue
			}
			declPiece, ok := pieceOf(obj.Pos())
			if !ok || declPiece == piece {
				continue
			}
			if !obj.Exported() {
				continue
			}
			if q, ok := pieceQualifier[declPiece]; ok {
				pos := fset.Position(id.Pos())
				edits[pos.Filename] = append(edits[pos.Filename], edit{pos.Offset, q + "."})
				counts["test->"+declPiece]++
			}
			continue
		}
		usePiece, ok := pieceOf(id.Pos())
		if !ok || (piece != "all" && usePiece != piece) {
			continue
		}
		declPiece, ok := pieceOf(obj.Pos())
		if !ok || declPiece == usePiece {
			continue
		}
		if !obj.Exported() {
			fmt.Println("UNEXPORTED-CROSS", usePiece, "->", declPiece, obj.Name())
			continue
		}
		// package-scope funcs/types/vars/consts only — skip methods
		// (kind "meth"), fields, locals.
		switch objKind(obj) {
		case "func", "type", "var", "const":
		default:
			continue
		}
		q, ok := pieceQualifier[declPiece]
		if !ok {
			continue
		}
		pos := fset.Position(id.Pos())
		edits[pos.Filename] = append(edits[pos.Filename], edit{pos.Offset, q + "."})
		counts[usePiece+"->"+declPiece]++
	}

	for fn, es := range edits {
		data, err := os.ReadFile(fn)
		if err != nil {
			panic(err)
		}
		sort.Slice(es, func(i, j int) bool { return es[i].off > es[j].off })
		// de-dup identical offsets (the same ident indexed twice can't
		// happen with Uses, but stay safe)
		seen := map[int]bool{}
		for _, e := range es {
			if seen[e.off] {
				continue
			}
			seen[e.off] = true
			data = append(data[:e.off], append([]byte(e.ins), data[e.off:]...)...)
		}
		if err := os.WriteFile(fn, data, 0o644); err != nil {
			panic(err)
		}
		fmt.Println("qualified", filepath.Base(fn), len(es), "uses")
	}
	var keys []string
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Println("EDGE", k, counts[k])
	}
	_ = strings.TrimSpace
}
