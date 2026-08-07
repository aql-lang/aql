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
func facadeFor(dir, out, qual string) error {
	mutable := map[string]bool{}
	switch qual {
	case "core":
		mutable = mutableVars
	case "check":
		mutable = map[string]bool{"DispatchBraid": true}
	case "compiler":
	default:
		return fmt.Errorf("unknown facade qualifier %q: want core, check or compiler", qual)
	}
	cold, err := coldSet(dir, out, qual)
	if err != nil {
		return err
	}
	return facadeGen(dir, out, qual, mutable, cold)
}

// coldSet derives the facade funcs NO caller in the repository reaches.
// They are emitted as func-value re-exports (var X = pkg.X) rather than
// wrapper funcs: same call syntax at every reference site, but no wrapper
// BODY to sit permanently uncovered under the ADR-008 gate. Deriving it
// beats a hand-kept list — every cut shifts which wrappers go cold.
func coldSet(dir, out, qual string) (map[string]bool, error) {
	repo := filepath.Dir(filepath.Dir(filepath.Clean(dir)))
	facade := filepath.Base(out)
	used := map[string]bool{}
	// Only the facade's CONSUMERS count: a call inside the source module
	// (or a sibling below it) reaches the real symbol, never the wrapper.
	// The scan is AST-based on purpose — a text scan counts identifiers in
	// COMMENTS and STRING LITERALS, which silently marks a cold wrapper
	// "used" and leaves its body uncovered under the ADR-008 gate.
	//
	// SCOPE matters as much as syntax. A BARE identifier reaches the wrapper
	// only from inside the facade's own package; every other package must
	// spell it `eng.X`. Counting bare identifiers everywhere was sound only
	// while the sibling alias tables re-exported the FACADE — with
	// `ApplyReach = eng.ApplyReach` in lang/go/native/aliases.go, lang's bare
	// `ApplyReach(...)` really did land on the wrapper. Once those tables
	// were repointed at the owning module (`= core.ApplyReach`), the same
	// bare call resolves locally and the wrapper became unreachable — but
	// still counted as used, so ~120 wrapper bodies stayed emitted and sat
	// permanently uncovered. Resolve by package, not by spelling.
	facadePkg := packageNameOf(filepath.Dir(out))
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
			// Outside the facade's own package a bare identifier cannot name
			// the wrapper, so only `<engImport>.X` counts. engImportName is
			// "" when the file does not import the facade package at all, in
			// which case nothing in it can reach a wrapper.
			inFacadePkg := f.Name.Name == facadePkg
			engName := ""
			if !inFacadePkg {
				engName = engImportName(f, facadePkg)
				if engName == "" {
					return nil
				}
			}
			var visit func(ast.Node) bool
			visit = func(n ast.Node) bool {
				// A qualified reference (pkg.Name) normally resolves to the
				// real symbol, so only the bare identifier reaches the
				// wrapper — EXCEPT when the qualifier is the facade package
				// itself, which is the only way an outside package can name
				// a wrapper at all.
				if sel, ok := n.(*ast.SelectorExpr); ok {
					if !inFacadePkg {
						if x, ok := sel.X.(*ast.Ident); ok && x.Name == engName {
							used[sel.Sel.Name] = true
							return false
						}
					}
					ast.Inspect(sel.X, visit)
					return false
				}
				// Outside the facade package a bare identifier names
				// something local, never a wrapper. Keep walking for nested
				// selectors, but record nothing.
				if !inFacadePkg {
					if _, ok := n.(*ast.Ident); ok {
						return false
					}
				}
				// A DECLARED name is not a reference. Consumers re-export
				// the facade wholesale (`Foo = eng.Foo` in basic's and
				// lang's aliases.go), and the left side of that spec is an
				// unqualified Ident spelled exactly like the wrapper — so
				// counting it marks every re-exported func "used" and leaves
				// the genuinely-uncalled wrappers as uncovered bodies. Walk
				// the parts that can REFERENCE, never the naming parts.
				switch d := n.(type) {
				case *ast.ValueSpec:
					if d.Type != nil {
						ast.Inspect(d.Type, visit)
					}
					for _, v := range d.Values {
						ast.Inspect(v, visit)
					}
					return false
				case *ast.TypeSpec:
					ast.Inspect(d.Type, visit)
					return false
				case *ast.FuncDecl:
					if d.Recv != nil {
						ast.Inspect(d.Recv, visit)
					}
					ast.Inspect(d.Type, visit)
					if d.Body != nil {
						ast.Inspect(d.Body, visit)
					}
					return false
				case *ast.Field:
					if d.Type != nil {
						ast.Inspect(d.Type, visit)
					}
					return false
				}
				if id, ok := n.(*ast.Ident); ok {
					used[id.Name] = true
				}
				return true
			}
			ast.Inspect(f, visit)
			return nil
		})
		if walkErr != nil {
			// An unreadable consumer tree means the "is it called?" answer
			// is unknown; treat every func as USED so nothing is demoted to
			// a re-export on incomplete evidence. Not an ignored failure —
			// the empty set IS the conservative answer.
			return map[string]bool{}, nil
		}
	}
	cold := map[string]bool{}
	cfg := &packages.Config{Mode: packages.NeedTypes | packages.NeedName, Dir: dir}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", dir, err)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("load %s: no packages", dir)
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
	return cold, nil
}

func facadeGen(coreDir, out, qual string, mutable, cold map[string]bool) error {
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedName,
		Dir:  coreDir,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return fmt.Errorf("load %s: %w", coreDir, err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		return fmt.Errorf("load %s: package has type errors", coreDir)
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
	"regexp"
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
		return fmt.Errorf("write %s: %w", out, err)
	}
	fmt.Printf("facade: %d types, %d consts, %d vars, %d funcs\n", len(types_), len(consts), len(vars), len(funcs))
	for _, s := range skipped {
		fmt.Println("FACADE-SKIP", s)
	}
	return nil
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

// packageNameOf reports the package clause of the first parsable .go file in
// dir. It is how coldSet learns the facade's own package name ("eng") rather
// than assuming it from the path, so a rename of the facade module does not
// silently turn every wrapper cold.
func packageNameOf(dir string) string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		f, perr := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, e.Name()), nil, parser.PackageClauseOnly)
		if perr != nil {
			continue
		}
		name := strings.TrimSuffix(f.Name.Name, "_test")
		if name != "" {
			return name
		}
	}
	return ""
}

// engImportName reports the local name a file uses for the facade package —
// the explicit alias when there is one, else the package's own name. It
// returns "" when the file does not import the facade at all, which is the
// signal that nothing in the file can reach a wrapper.
func engImportName(f *ast.File, facadePkg string) string {
	for _, im := range f.Imports {
		path := strings.Trim(im.Path.Value, `"`)
		if filepath.Base(path) != "go" && filepath.Base(path) != facadePkg {
			continue
		}
		// The module is .../<facadePkg>/go, so the directory ABOVE the
		// trailing "go" carries the name.
		base := filepath.Base(path)
		if base == "go" {
			base = filepath.Base(filepath.Dir(path))
		}
		if base != facadePkg {
			continue
		}
		if im.Name != nil {
			return im.Name.Name
		}
		return facadePkg
	}
	return ""
}
