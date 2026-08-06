package main

// facade mode: generate the eng-side facade for the core module — type
// aliases, const re-exports, set-once var mirrors, and thin inlinable
// wrapper funcs. Mutable slot vars are excluded (their writers use the
// qualified form).
//
//	piecetool -facade <core module dir> <output file>

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// mutableVars are written after init (slot tables); eng-side writers
// use core.X directly, so the facade must not mirror them.
var mutableVars = map[string]bool{
	"AnalysisImpl": true, "CheckBraid": true, "JoinCarriersHook": true,
	"DriftWindowRecorder": true, "NewEmitStateHook": true, "NewIsolatedEmitHook": true,
}

// facadeFor parameterizes the generation per source module: the
// package qualifier, the mutable slot vars never mirrored, and the
// cold set emitted as func-value re-exports.
func facadeFor(dir, out, qual string) {
	mutable := map[string]bool{}
	switch qual {
	case "core":
		mutable = mutableVars
	case "check":
		mutable = map[string]bool{"DispatchBraid": true}
	case "compiler":
	default:
		panic("unknown facade qualifier " + qual)
	}
	facadeGen(dir, out, qual, mutable, coldSet(dir, out, qual))
}

// coldSet derives the facade funcs NO caller in the repository reaches.
// They are emitted as func-value re-exports (var X = pkg.X) rather than
// wrapper funcs: same call syntax at every reference site, but no wrapper
// BODY to sit permanently uncovered under the ADR-008 gate. Deriving it
// beats a hand-kept list — every cut shifts which wrappers go cold.
func coldSet(dir, out, qual string) map[string]bool {
	repo := filepath.Dir(filepath.Dir(filepath.Clean(dir)))
	facade := filepath.Base(out)
	used := map[string]bool{}
	// Only the facade's CONSUMERS count: a call inside the source module
	// (or a sibling below it) reaches the real symbol, never the wrapper.
	// The scan is AST-based on purpose — a text scan counts identifiers in
	// COMMENTS and STRING LITERALS, which silently marks a cold wrapper
	// "used" and leaves its body uncovered under the ADR-008 gate.
	consumers := []string{"eng", "basic", "lang", "cmd", "calc", "wpg", "test", "utils"}
	for _, c := range consumers {
		root := filepath.Join(repo, c)
		if _, err := os.Stat(root); err != nil {
			continue
		}
		walkErr := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				switch d.Name() {
				case ".git", "node_modules", "coverage", "bin", "out":
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || filepath.Base(p) == facade {
				return nil
			}
			fset := token.NewFileSet()
			f, perr := parser.ParseFile(fset, p, nil, 0)
			if perr != nil {
				return nil
			}
			ast.Inspect(f, func(n ast.Node) bool {
				// A qualified reference (pkg.Name) resolves to the real
				// symbol; only the bare identifier reaches the wrapper.
				if sel, ok := n.(*ast.SelectorExpr); ok {
					ast.Inspect(sel.X, func(inner ast.Node) bool {
						if id, ok := inner.(*ast.Ident); ok {
							used[id.Name] = true
						}
						return true
					})
					return false
				}
				if id, ok := n.(*ast.Ident); ok {
					used[id.Name] = true
				}
				return true
			})
			return nil
		})
		if walkErr != nil {
			// An unreadable consumer tree means the "is it called?" answer
			// is unknown; treat every func as USED so nothing is demoted to
			// a re-export on incomplete evidence.
			return map[string]bool{}
		}
	}
	cold := map[string]bool{}
	cfg := &packages.Config{Mode: packages.NeedTypes | packages.NeedName, Dir: dir}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil || len(pkgs) == 0 {
		return cold
	}
	scope := pkgs[0].Types.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		if _, isFn := obj.(*types.Func); !isFn {
			continue
		}
		if !used[name] {
			cold[name] = true
		}
	}
	return cold
}

func facadeGen(coreDir, out, qual string, mutable, cold map[string]bool) {
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedName,
		Dir:  coreDir,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		panic(err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		os.Exit(1)
	}
	pkg := pkgs[0].Types
	scope := pkg.Scope()

	qualifier := func(other *types.Package) string {
		if other == pkg {
			return qual
		}
		return other.Name()
	}

	var types_, consts, vars, funcs, coldOut, skipped []string
	names := scope.Names()
	sort.Strings(names)
	for _, name := range names {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		switch o := obj.(type) {
		case *types.TypeName:
			if o.IsAlias() {
				types_ = append(types_, fmt.Sprintf("\t%s = %s.%s", name, qual, name))
			} else {
				types_ = append(types_, fmt.Sprintf("\t%s = %s.%s", name, qual, name))
			}
		case *types.Const:
			consts = append(consts, fmt.Sprintf("\t%s = %s.%s", name, qual, name))
		case *types.Var:
			if mutable[name] {
				skipped = append(skipped, "mutable var "+name)
				continue
			}
			vars = append(vars, fmt.Sprintf("\t%s = %s.%s", name, qual, name))
		case *types.Func:
			sig := o.Type().(*types.Signature)
			if sig.TypeParams().Len() > 0 {
				skipped = append(skipped, "generic func "+name)
				continue
			}
			if usesUnexported(sig) {
				skipped = append(skipped, "func w/ unexported types "+name)
				continue
			}
			if cold[name] {
				coldOut = append(coldOut, fmt.Sprintf("\t%s = %s.%s", name, qual, name))
				continue
			}
			params, args := renderParams(sig, qualifier)
			results := renderResults(sig, qualifier)
			ret := "return "
			if sig.Results().Len() == 0 {
				ret = ""
			}
			funcs = append(funcs, fmt.Sprintf("func %s(%s)%s { %s%s.%s(%s) }", name, params, results, ret, qual, name, args))
		}
	}

	var b strings.Builder
	b.WriteString(`// Code generated by piecetool -facade; DO NOT EDIT.
//
// The eng facade over the core module (design/ENG-FOUR-PIECE.0.md
// Stage 4): type aliases are zero-cost, the function wrappers are thin
// enough to inline, and set-once vars mirror by value at init (core's
// package init completes first). Mutable slot tables are NOT mirrored —
// their installers write core.X directly.
package eng

import (
	"context"
	"io"
	"math/big"
	"time"

	check "github.com/boru-lang/boru/check/go"
	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
	apd "github.com/cockroachdb/apd/v3"
)

var _ = context.Canceled
var _ io.Reader
var _ big.Int
var _ regexp.Regexp
var _ time.Time
var _ apd.Decimal
var _ = core.TheInactiveEmit
var _ = check.DispatchBraid
var _ compiler.EmitState

`)
	b.WriteString("type (\n" + strings.Join(types_, "\n") + "\n)\n\n")
	if len(consts) > 0 {
		b.WriteString("const (\n" + strings.Join(consts, "\n") + "\n)\n\n")
	}
	if len(vars) > 0 {
		b.WriteString("var (\n" + strings.Join(vars, "\n") + "\n)\n\n")
	}
	if len(coldOut) > 0 {
		b.WriteString(`// Cold func-value re-exports: exported core funcs no suite calls
// through the facade (API surface only — lang re-export vars,
// short-circuited operands, external embedders). A var of func type
// keeps every reference site compiling verbatim with no wrapper body
// to leave uncovered; a name moves back up to a wrapper func when a
// hot path starts calling it.
var (
` + strings.Join(coldOut, "\n") + "\n)\n\n")
	}
	b.WriteString(strings.Join(funcs, "\n\n") + "\n")
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("facade: %d types, %d consts, %d vars, %d funcs\n", len(types_), len(consts), len(vars), len(funcs))
	for _, s := range skipped {
		fmt.Println("FACADE-SKIP", s)
	}
}

func usesUnexported(sig *types.Signature) bool {
	bad := false
	check := func(t types.Type) {
		types.TypeString(t, func(p *types.Package) string { return p.Name() })
		var walk func(types.Type)
		seen := map[types.Type]bool{}
		walk = func(t types.Type) {
			if seen[t] {
				return
			}
			seen[t] = true
			switch tt := t.(type) {
			case *types.Named:
				if tt.Obj().Pkg() != nil && !tt.Obj().Exported() {
					bad = true
				}
			case *types.Pointer:
				walk(tt.Elem())
			case *types.Slice:
				walk(tt.Elem())
			case *types.Array:
				walk(tt.Elem())
			case *types.Map:
				walk(tt.Key())
				walk(tt.Elem())
			case *types.Chan:
				walk(tt.Elem())
			case *types.Signature:
				for i := 0; i < tt.Params().Len(); i++ {
					walk(tt.Params().At(i).Type())
				}
				for i := 0; i < tt.Results().Len(); i++ {
					walk(tt.Results().At(i).Type())
				}
			}
		}
		walk(t)
	}
	for i := 0; i < sig.Params().Len(); i++ {
		check(sig.Params().At(i).Type())
	}
	for i := 0; i < sig.Results().Len(); i++ {
		check(sig.Results().At(i).Type())
	}
	return bad
}

func renderParams(sig *types.Signature, qual types.Qualifier) (decl, args string) {
	var ds, as []string
	n := sig.Params().Len()
	for i := 0; i < n; i++ {
		p := sig.Params().At(i)
		name := fmt.Sprintf("a%d", i)
		ts := types.TypeString(p.Type(), qual)
		if sig.Variadic() && i == n-1 {
			ts = "..." + strings.TrimPrefix(ts, "[]")
			as = append(as, name+"...")
		} else {
			as = append(as, name)
		}
		ds = append(ds, name+" "+ts)
	}
	return strings.Join(ds, ", "), strings.Join(as, ", ")
}

func renderResults(sig *types.Signature, qual types.Qualifier) string {
	n := sig.Results().Len()
	if n == 0 {
		return ""
	}
	var rs []string
	for i := 0; i < n; i++ {
		rs = append(rs, types.TypeString(sig.Results().At(i).Type(), qual))
	}
	if n == 1 {
		return " " + rs[0]
	}
	return " (" + strings.Join(rs, ", ") + ")"
}
