package modules

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"unsafe"

	"github.com/aql-lang/aql/eng/go/stackform"
	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/lang/go/native/help"
)

// moduleNamesFn indirects the package-level Names() so the `debug-modules`
// handler can list available modules without the `modules` map's
// initializer depending on BuildDebugModule depending back on `modules`
// (a static init cycle). Assigned in init() — not a var initializer —
// because init funcs are outside the var-init dependency graph, mirroring
// vm.go's resolveFn.
var moduleNamesFn func() []string

func init() { moduleNamesFn = Names }

// BuildDebugModule creates the "aql:debug" native module — the curated
// front door for debugging an AQL program: printing taps, structural and
// system introspection, value sizing, and performance measurement.
//
// It follows the standard native-module shape (an isolated sub-registry of
// Go natives, exported as trivial-delegation FnDef wrappers under the
// "Debug" namespace). The design is design/DEBUG-MODULE.0.md; this builds
// the in-process surfaces (Phases 1–4). The remote surfaces (§7 — live
// dashboard, attach, serverless channel) are gated on the unbuilt
// Service/Process layer and are intentionally not implemented here.
//
//	import "aql:debug"
//	(compute) Debug.tap further-process     # print + pass through
//	[1 add 2 mul 3] Debug.steps             # engine step count (deterministic)
//	[heavy] 100 Debug.bench                 # {n mean-ms min-ms max-ms ...}
//	big-value Debug.sizeof                  # estimated retained bytes
//	"add" Debug.explain                     # the describe text as a String
func BuildDebugModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}

	natives := append(debugNatives(), stepNatives()...)
	for _, n := range natives {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()
	for _, n := range natives {
		exports.Set(debugExportName(n.Name), makeModuleFnDef(n, subReg))
	}

	return native.ModuleDesc{
		ID:      parent.Modules.NextID(),
		Exports: map[string]*native.OrderedMap{"Debug": exports},
	}, nil
}

// debugExportName strips the internal "debug-" prefix off an inner native
// name to produce the dotted export key (debug-tap -> "tap").
func debugExportName(internal string) string {
	return strings.TrimPrefix(internal, "debug-")
}

// debugNatives builds every Go-implemented debug primitive. Each uses
// BarrierPos: -1 so the dotted swap form (`value Debug.tap`) dispatches
// (the module-wrapper rule, lang/go/CLAUDE.md). Body-taking words mark
// NoEvalArgs so the quoted code body reaches the handler unevaluated.
func debugNatives() []native.NativeFunc {
	return []native.NativeFunc{
		// ── (A) Printing & tracing ────────────────────────────────────
		{
			Name: "debug-tap",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TAny},
				Returns:    []*native.Type{native.TAny},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					fmt.Fprintln(r.Output, native.FormatForPrint(args[0]))
					return []native.Value{args[0]}, nil
				},
			}},
		},
		{
			// `value label Debug.label` — print "label: value", return value.
			Name: "debug-label",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TAny, native.TString},
				Returns:    []*native.Type{native.TAny},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					label, err := args[1].AsConcreteString()
					if err != nil {
						return nil, err
					}
					fmt.Fprintf(r.Output, "%s: %s\n", label, native.FormatForPrint(args[0]))
					return []native.Value{args[0]}, nil
				},
			}},
		},
		{
			// Like tap, but prints the type alongside the value.
			Name: "debug-dump",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TAny},
				Returns:    []*native.Type{native.TAny},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					v := args[0]
					typeName := "Any"
					if v.Parent != nil {
						typeName = v.Parent.String()
					}
					fmt.Fprintf(r.Output, "%s = %s\n", typeName, native.FormatForPrint(v))
					return []native.Value{v}, nil
				},
			}},
		},
		{
			// `cond msg Debug.assert` — raise assertion_failure when cond is false.
			Name: "debug-assert",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TBoolean, native.TString},
				Returns:    []*native.Type{},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					cond, err := args[0].AsConcreteBoolean()
					if err != nil {
						return nil, err
					}
					msg, err := args[1].AsConcreteString()
					if err != nil {
						return nil, err
					}
					if !cond {
						return nil, r.AqlError("assertion_failure", msg, "Debug.assert")
					}
					return nil, nil
				},
			}},
		},
		{
			// A typed hole: always raises not_implemented with the message.
			Name: "debug-todo",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TString},
				Returns:    []*native.Type{native.TNever},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					msg, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					return nil, r.AqlError("not_implemented", msg, "Debug.todo")
				},
			}},
		},

		// ── (C) Program structural analysis ───────────────────────────
		{
			// Parse source to its token/value list without running it.
			Name: "debug-parse",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TString},
				Returns:    []*native.Type{native.TList},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					src, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					if r.ParseFunc == nil {
						return nil, r.AqlError("debug_error", "Debug.parse: parser not configured", "Debug.parse")
					}
					tokens, perr := r.ParseFunc(src)
					if perr != nil {
						return nil, r.AqlError("parse_error", fmt.Sprintf("Debug.parse: %v", perr), "Debug.parse")
					}
					lst := native.NewList(tokens)
					lst.Quoted = true
					return []native.Value{lst}, nil
				},
			}},
		},
		{
			// The distinct word names a quoted body references.
			Name: "debug-deps",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TList},
				Returns:    []*native.Type{native.TList},
				NoEvalArgs: map[int]bool{0: true},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					body, err := native.RequireConcreteList(args[0], "Debug.deps")
					if err != nil {
						return nil, err
					}
					seen := map[string]bool{}
					var names []string
					native.WalkBodyWords(body.Slice(), func(w native.WordInfo, _ native.Value) {
						if w.Name == "" || seen[w.Name] {
							return
						}
						seen[w.Name] = true
						names = append(names, w.Name)
					})
					sort.Strings(names)
					return []native.Value{stringsToList(names)}, nil
				},
			}},
		},
		{
			// The full `describe` text for a word, as a String.
			Name: "debug-explain",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TString},
				Returns:    []*native.Type{native.TString},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					name, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					var sb strings.Builder
					native.DescribeName(r, &sb, name)
					return []native.Value{native.NewString(strings.TrimRight(sb.String(), "\n"))}, nil
				},
			}},
		},

		// ── (D) System structural analysis ────────────────────────────
		{
			// Every documented built-in word name.
			Name: "debug-words",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{},
				Returns:    []*native.Type{native.TList},
				BarrierPos: -1,
				Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					words := append([]string(nil), help.Words()...)
					sort.Strings(words)
					return []native.Value{stringsToList(words)}, nil
				},
			}},
		},
		{
			// Current def-bound names mapped to their active top binding.
			Name: "debug-defs",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{},
				Returns:    []*native.Type{native.TMap},
				BarrierPos: -1,
				Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					names := append([]string(nil), r.Defs.Names()...)
					sort.Strings(names)
					om := native.NewOrderedMap()
					for _, name := range names {
						if v, ok := r.Defs.Top(name); ok {
							om.Set(name, v)
						}
					}
					return []native.Value{native.NewMap(om)}, nil
				},
			}},
		},
		{
			// The native modules available to import.
			Name: "debug-modules",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{},
				Returns:    []*native.Type{native.TList},
				BarrierPos: -1,
				Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					names := moduleNamesFn()
					sort.Strings(names)
					out := make([]string, len(names))
					for i, n := range names {
						out[i] = "aql:" + n
					}
					return []native.Value{stringsToList(out)}, nil
				},
			}},
		},

		// ── (E) Memory analysis ───────────────────────────────────────
		{
			// Estimated retained byte size of a value (deep walk).
			Name: "debug-sizeof",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TAny},
				Returns:    []*native.Type{native.TInteger},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewInteger(int64(sizeOfValue(args[0])))}, nil
				},
			}},
		},
		{
			// Structural census of a value: counts by kind, depth, node count.
			Name: "debug-shape",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TAny},
				Returns:    []*native.Type{native.TMap},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					var c shapeCensus
					c.walk(args[0], 0)
					om := native.NewOrderedMap()
					om.Set("nodes", native.NewInteger(int64(c.nodes)))
					om.Set("lists", native.NewInteger(int64(c.lists)))
					om.Set("maps", native.NewInteger(int64(c.maps)))
					om.Set("strings", native.NewInteger(int64(c.strings)))
					om.Set("scalars", native.NewInteger(int64(c.scalars)))
					om.Set("max-depth", native.NewInteger(int64(c.maxDepth)))
					return []native.Value{native.NewMap(om)}, nil
				},
			}},
		},

		// ── (F) Performance analysis ──────────────────────────────────
		{
			// Engine step count for a body (deterministic, clock-free).
			Name: "debug-steps",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TList},
				Returns:    []*native.Type{native.TInteger},
				NoEvalArgs: map[int]bool{0: true},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					_, steps, err := runCounted(r, args[0], "Debug.steps")
					if err != nil {
						return nil, err
					}
					return []native.Value{native.NewInteger(int64(steps))}, nil
				},
			}},
		},
		{
			// Run once, return {result, elapsed-ms, steps}.
			Name: "debug-time",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TList},
				Returns:    []*native.Type{native.TMap},
				NoEvalArgs: map[int]bool{0: true},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					clk := native.EffectiveClock(r)
					start := clk.Now()
					res, steps, err := runCounted(r, args[0], "Debug.time")
					elapsed := clk.Now().Sub(start)
					if err != nil {
						return nil, err
					}
					om := native.NewOrderedMap()
					om.Set("result", lastOrNone(res))
					om.Set("elapsed-ms", native.NewFloat(float64(elapsed.Nanoseconds())/1e6))
					om.Set("steps", native.NewInteger(int64(steps)))
					return []native.Value{native.NewMap(om)}, nil
				},
			}},
		},
		{
			// `body n Debug.bench` — run n times, return timing stats.
			Name: "debug-bench",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TList, native.TInteger},
				Returns:    []*native.Type{native.TMap},
				NoEvalArgs: map[int]bool{0: true},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					n, err := args[1].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					if n <= 0 {
						return nil, r.AqlError("debug_error",
							fmt.Sprintf("Debug.bench: n (%d) must be > 0", n), "Debug.bench")
					}
					clk := native.EffectiveClock(r)
					var total, minMs, maxMs float64
					var steps int
					for i := int64(0); i < n; i++ {
						start := clk.Now()
						_, s, rerr := runCounted(r, args[0], "Debug.bench")
						ms := float64(clk.Now().Sub(start).Nanoseconds()) / 1e6
						if rerr != nil {
							return nil, rerr
						}
						steps = s
						total += ms
						if i == 0 || ms < minMs {
							minMs = ms
						}
						if i == 0 || ms > maxMs {
							maxMs = ms
						}
					}
					om := native.NewOrderedMap()
					om.Set("n", native.NewInteger(n))
					om.Set("total-ms", native.NewFloat(total))
					om.Set("mean-ms", native.NewFloat(total/float64(n)))
					om.Set("min-ms", native.NewFloat(minMs))
					om.Set("max-ms", native.NewFloat(maxMs))
					om.Set("steps-per-run", native.NewInteger(int64(steps)))
					return []native.Value{native.NewMap(om)}, nil
				},
			}},
		},
		{
			// Run a body with step-by-step tracing printed to output.
			Name: "debug-trace",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TList},
				Returns:    []*native.Type{native.TAny},
				NoEvalArgs: map[int]bool{0: true},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					body, err := native.RequireConcreteList(args[0], "Debug.trace")
					if err != nil {
						return nil, err
					}
					res, terr := native.RunTrace(r, append([]native.Value(nil), body.Slice()...), r.Output)
					if terr != nil {
						return nil, terr
					}
					return []native.Value{lastOrNone(res)}, nil
				},
			}},
		},
		{
			// Per-word step-count profile of a body, costliest first.
			Name: "debug-profile",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TList},
				Returns:    []*native.Type{native.TList},
				NoEvalArgs: map[int]bool{0: true},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					body, err := native.RequireConcreteList(args[0], "Debug.profile")
					if err != nil {
						return nil, err
					}
					counts := map[string]int{}
					sub := native.New(r)
					sub.SetTrace(func(_ int, pointer int, stack []native.Value, _ string) {
						if pointer < 0 || pointer >= len(stack) {
							return
						}
						v := stack[pointer]
						if !native.IsWord(v) {
							return
						}
						w, werr := native.AsWord(v)
						if werr != nil || w.Name == "" {
							return
						}
						counts[w.Name]++
					})
					if _, rerr := sub.Run(append([]native.Value(nil), body.Slice()...)); rerr != nil {
						return nil, rerr
					}
					return []native.Value{profileRows(counts)}, nil
				},
			}},
		},

		// ── (C2) Word reflection ──────────────────────────────────────
		{
			// The signatures of a word, as structured data.
			Name: "debug-sig",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TString},
				Returns:    []*native.Type{native.TList},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					name, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					fn := r.Lookup(name)
					if fn == nil {
						return nil, r.AqlError("debug_error",
							fmt.Sprintf("Debug.sig: no such word %q", name), "Debug.sig")
					}
					var sigs []native.Value
					for _, sig := range fn.Signatures {
						sm := native.NewOrderedMap()
						sm.Set("args", typeLeavesToList(sig.ArgTypes()))
						sm.Set("returns", typeLeavesToList(sig.Returns))
						sigs = append(sigs, native.NewMap(sm))
					}
					return []native.Value{native.NewList(sigs)}, nil
				},
			}},
		},
		{
			// The quoted body of an AQL-defined word; `native/q` for a host word.
			Name: "debug-body",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TString},
				Returns:    []*native.Type{native.TAny},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					name, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					fn := r.Lookup(name)
					if fn == nil {
						return nil, r.AqlError("debug_error",
							fmt.Sprintf("Debug.body: no such word %q", name), "Debug.body")
					}
					for _, sig := range fn.OwnSigs() {
						if len(sig.Body) > 0 {
							body := native.NewList(append([]native.Value(nil), sig.Body...))
							body.Quoted = true
							return []native.Value{body}, nil
						}
					}
					return []native.Value{native.NewAtom("native")}, nil
				},
			}},
		},
		{
			// Print a name's current binding and return it (None if unbound).
			Name: "debug-watch",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TString},
				Returns:    []*native.Type{native.TAny},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					name, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					v, ok := r.Defs.Top(name)
					if !ok {
						v = native.NewNone()
					}
					fmt.Fprintf(r.Output, "%s = %s\n", name, native.FormatForPrint(v))
					return []native.Value{v}, nil
				},
			}},
		},
		{
			// Compile a quoted body to its StackForm disassembly (a String).
			Name: "debug-disasm",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TList},
				Returns:    []*native.Type{native.TString},
				NoEvalArgs: map[int]bool{0: true},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					body, err := native.RequireConcreteList(args[0], "Debug.disasm")
					if err != nil {
						return nil, err
					}
					sub, derr := native.DefaultRegistry()
					if derr != nil {
						return nil, derr
					}
					_, form, cerr := stackform.Compile(sub, append([]native.Value(nil), body.Slice()...))
					if cerr != nil {
						return nil, r.AqlError("debug_error",
							fmt.Sprintf("Debug.disasm: %v", cerr), "Debug.disasm")
					}
					return []native.Value{native.NewString(strings.TrimRight(stackform.Pretty(form), "\n"))}, nil
				},
			}},
		},

		{
			// A snapshot of the current data stack at the call site.
			Name: "debug-stack",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{},
				Returns:    []*native.Type{native.TList},
				BarrierPos: -1,
				Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					snap, ok := r.CurrentStack()
					if !ok {
						return []native.Value{native.NewList(nil)}, nil
					}
					return []native.Value{native.NewList(snap)}, nil
				},
			}},
		},

		// ── (E2) Runtime memory (host) ────────────────────────────────
		{
			// Go-runtime heap stats as a map.
			Name: "debug-heap",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{},
				Returns:    []*native.Type{native.TMap},
				BarrierPos: -1,
				Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					var m runtime.MemStats
					runtime.ReadMemStats(&m)
					return []native.Value{heapMap(&m)}, nil
				},
			}},
		},
		{
			// Force a GC and report before/after heap deltas.
			Name: "debug-gc",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{},
				Returns:    []*native.Type{native.TMap},
				BarrierPos: -1,
				Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					var before, after runtime.MemStats
					runtime.ReadMemStats(&before)
					runtime.GC()
					runtime.ReadMemStats(&after)
					om := native.NewOrderedMap()
					om.Set("alloc-before", native.NewInteger(int64(before.Alloc)))
					om.Set("alloc-after", native.NewInteger(int64(after.Alloc)))
					om.Set("num-gc", native.NewInteger(int64(after.NumGC)))
					return []native.Value{native.NewMap(om)}, nil
				},
			}},
		},
	}
}

// typeLeavesToList renders a slice of types as a List of their leaf names.
func typeLeavesToList(types []*native.Type) native.Value {
	out := make([]native.Value, 0, len(types))
	for _, t := range types {
		if t == nil {
			continue
		}
		out = append(out, native.NewString(t.Leaf()))
	}
	return native.NewList(out)
}

// heapMap renders runtime.MemStats as the Debug.heap result map.
func heapMap(m *runtime.MemStats) native.Value {
	om := native.NewOrderedMap()
	om.Set("alloc", native.NewInteger(int64(m.Alloc)))
	om.Set("total-alloc", native.NewInteger(int64(m.TotalAlloc)))
	om.Set("heap-objects", native.NewInteger(int64(m.HeapObjects)))
	om.Set("num-gc", native.NewInteger(int64(m.NumGC)))
	return native.NewMap(om)
}

// runCounted runs a quoted body (args[0]) in a fresh sub-engine with a
// step-counting trace hook installed, returning the residual stack and
// the number of engine steps executed. The body list must be concrete.
func runCounted(parent *native.Registry, bodyVal native.Value, op string) ([]native.Value, int, error) {
	body, err := native.RequireConcreteList(bodyVal, op)
	if err != nil {
		return nil, 0, err
	}
	steps := 0
	sub := native.New(parent)
	sub.SetTrace(func(_ int, _ int, _ []native.Value, _ string) { steps++ })
	res, rerr := sub.Run(append([]native.Value(nil), body.Slice()...))
	if rerr != nil {
		return nil, 0, rerr
	}
	return res, steps, nil
}

// lastOrNone returns the last value of a residual stack, or None when empty.
func lastOrNone(res []native.Value) native.Value {
	if len(res) == 0 {
		return native.NewNone()
	}
	return res[len(res)-1]
}

// stringsToList builds a List value from a slice of strings.
func stringsToList(ss []string) native.Value {
	out := make([]native.Value, len(ss))
	for i, s := range ss {
		out[i] = native.NewString(s)
	}
	return native.NewList(out)
}

// profileRows converts a per-word count map into a List of
// {word, steps} maps sorted by descending step count (ties by name).
func profileRows(counts map[string]int) native.Value {
	type row struct {
		word  string
		steps int
	}
	rows := make([]row, 0, len(counts))
	for w, n := range counts {
		rows = append(rows, row{w, n})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].steps != rows[j].steps {
			return rows[i].steps > rows[j].steps
		}
		return rows[i].word < rows[j].word
	})
	out := make([]native.Value, len(rows))
	for i, rw := range rows {
		om := native.NewOrderedMap()
		om.Set("word", native.NewString(rw.word))
		om.Set("steps", native.NewInteger(int64(rw.steps)))
		out[i] = native.NewMap(om)
	}
	return native.NewList(out)
}

// sizeOfValue estimates the retained byte size of an AQL value by a deep
// walk. The estimate is heuristic (per-node overhead + payload bytes) but
// deterministic, so it is useful for relative comparisons. Type literals
// and carriers (non-concrete) contribute only their node overhead — the
// walk never calls a list/map accessor on a non-concrete value, so it
// cannot panic on them.
func sizeOfValue(v native.Value) int {
	const nodeOverhead = int(unsafe.Sizeof(native.Value{}))
	size := nodeOverhead
	if !native.IsConcrete(v) {
		return size
	}
	switch {
	case isStringValue(v):
		if s, err := v.AsConcreteString(); err == nil {
			size += len(s)
		}
	case isListValue(v):
		if lst, lerr := native.AsList(v); lerr == nil && !lst.IsNil() {
			for _, e := range lst.Slice() {
				size += sizeOfValue(e)
			}
		}
	case isMapValue(v):
		if m, merr := native.AsMap(v); merr == nil && m != nil {
			for _, k := range m.Keys() {
				size += len(k)
				if ev, has := m.Get(k); has {
					size += sizeOfValue(ev)
				}
			}
		}
	}
	return size
}

// shapeCensus accumulates a structural census during a value walk.
type shapeCensus struct {
	nodes, lists, maps, strings, scalars, maxDepth int
}

func (c *shapeCensus) walk(v native.Value, depth int) {
	c.nodes++
	if depth > c.maxDepth {
		c.maxDepth = depth
	}
	switch {
	case isListValue(v):
		c.lists++
		if lst, lerr := native.AsList(v); lerr == nil && !lst.IsNil() {
			for _, e := range lst.Slice() {
				c.walk(e, depth+1)
			}
		}
	case isMapValue(v):
		c.maps++
		if m, merr := native.AsMap(v); merr == nil && m != nil {
			for _, k := range m.Keys() {
				if ev, has := m.Get(k); has {
					c.walk(ev, depth+1)
				}
			}
		}
	case isStringValue(v):
		c.strings++
	default:
		c.scalars++
	}
}

// isStringValue / isListValue / isMapValue classify a CONCRETE value by
// kind via v.Is (Behavior.Match), so string/list/map LITERAL subtypes
// (e.g. ProperString, whose Parent is not exactly TString) are recognised.
// The IsConcrete guard means carriers and bare type literals (non-concrete,
// even though they share the family Parent) are classified as plain nodes
// and never reach an accessor — the no-panic discipline.
func isStringValue(v native.Value) bool {
	return native.IsConcrete(v) && v.Is(native.TString)
}

func isListValue(v native.Value) bool {
	return native.IsConcrete(v) && v.Is(native.TList)
}

func isMapValue(v native.Value) bool {
	return native.IsConcrete(v) && v.Is(native.TMap)
}
