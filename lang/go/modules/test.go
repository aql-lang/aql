package modules

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// testRun is the per-registry state that test/describe/it accumulate
// into. It is created lazily on first call to a test word and stored
// under capTestRun on the parent (caller's) registry — so successive
// test calls from the same Run() append to the same set, and
// `test.results` returns everything seen so far.
type testRun struct {
	mu       sync.Mutex
	path     []string       // active describe stack
	results  []native.Value // TestResult records, accumulated in order
	failures int            // count of failed test cases
}

const capTestRun = "test.run.active"

// testParseOnce caches the parsed AQL preamble that defines the
// TestCase / TestSet / TestSpec / TestResult record types plus the
// pure-AQL spec runner. The preamble is parsed once per process and
// reused for every BuildTestModule call.
var (
	testParseOnce sync.Once
	testParsed    []native.Value
	testParseErr  error
)

// BuildTestModule creates the "aql:test" native module. The module
// is intentionally hybrid:
//
//   - The imperative API (test / describe / it / assert.*) is
//     implemented in Go because it needs to manage the active
//     testRun, catch errors, and time execution.
//   - The declarative pieces (TestCase, TestSet, TestSpec, TestResult
//     record types, plus run-spec) live in AQL because they are pure
//     data construction and benefit from reading like a schema.
//
// Both are folded into the `test` and `assert` exports so callers
// get one import and two dotted namespaces.
func BuildTestModule(parent *native.Registry) (native.ModuleDesc, error) {
	if parent.ParseFunc == nil {
		return native.ModuleDesc{}, fmt.Errorf("test: parser not configured")
	}

	testParseOnce.Do(func() {
		testParsed, testParseErr = parent.ParseFunc(testAQLPreamble)
	})
	if testParseErr != nil {
		return native.ModuleDesc{}, fmt.Errorf("test: parse preamble: %w", testParseErr)
	}

	// Build the module sub-registry, register the Go-implemented test
	// natives into it, then run the AQL preamble so record types and
	// spec-runner fns are defined alongside them. The preamble's
	// `export` call assembles the final export map.
	modReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, fmt.Errorf("test: init: %w", err)
	}
	modReg.Output = parent.Output
	modReg.ErrOutput = parent.ErrOutput
	modReg.Input = parent.Input
	modReg.ParseFunc = parent.ParseFunc
	modReg.BaseDir = parent.BaseDir

	if parent.Modules.InitFunc != nil {
		parent.Modules.InitFunc(modReg)
		modReg.Modules.InitFunc = parent.Modules.InitFunc
	} else {
		native.Register(modReg)
	}

	for _, n := range testNatives(parent) {
		modReg.RegisterNativeFunc(n)
	}

	// Run the preamble. We reuse RunModuleBody's machinery via a
	// minimal local exporter (we cannot use RunModuleBody itself —
	// it builds a fresh modReg that doesn't see our natives).
	exports := map[string]*native.OrderedMap{}
	modReg.RegisterNativeFunc(native.NativeFunc{
		Name: "export",
		Signatures: []native.NativeSig{
			{
				Args: []*native.Type{native.TAtom, native.TMap},
				Handler: func(eargs []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					name, _ := eargs[0].AsConcreteAtom()
					return resolveExport(modReg, exports, name, eargs[1])
				},
				Returns: []*native.Type{}, BarrierPos: -1,
			},
			{
				Args: []*native.Type{native.TString, native.TMap},
				Handler: func(eargs []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					name, _ := eargs[0].AsConcreteString()
					return resolveExport(modReg, exports, name, eargs[1])
				},
				Returns: []*native.Type{}, BarrierPos: -1,
			},
		},
	})

	tokens := append([]native.Value(nil), testParsed...)
	sub := native.New(modReg)
	if _, err := sub.Run(tokens); err != nil {
		return native.ModuleDesc{}, fmt.Errorf("test: run preamble: %w", err)
	}

	return native.ModuleDesc{
		ID:      parent.Modules.NextID(),
		Exports: exports,
	}, nil
}

// resolveExport collects exports from an `export "name" {...}` call,
// resolving each value through the module registry so word references
// pick up the actual bound type / fn / Go-native FnDef. This mirrors
// what RunModuleBody does internally but is duplicated here because
// BuildTestModule manages its own modReg.
func resolveExport(modReg *native.Registry, exports map[string]*native.OrderedMap, name string, mapArg native.Value) ([]native.Value, error) {
	if !native.IsConcrete(mapArg) {
		return nil, fmt.Errorf("test/export: value must be a concrete map")
	}
	rawMap, _ := native.AsMap(mapArg)
	resolved := native.NewOrderedMap()
	for _, key := range rawMap.Keys() {
		val, _ := rawMap.Get(key)
		resolved.Set(key, resolveTestExport(modReg, val))
	}
	exports[name] = resolved
	return nil, nil
}

// resolveTestExport mirrors native.resolveModuleExport but is local
// to this package — the kernel helper is unexported.
func resolveTestExport(modReg *native.Registry, v native.Value) native.Value {
	var name string
	switch {
	case native.IsWord(v):
		w, _ := native.AsWord(v)
		name = w.Name
	case v.Parent.Matches(native.TString):
		name, _ = native.AsString(v)
	case native.IsAtom(v):
		name, _ = native.AsAtom(v)
	default:
		return v
	}
	if tv, ok := modReg.TopTypeBody(name); ok {
		if fnDef, ok := tv.Data.(native.FnDefInfo); ok && fnDef.Registry == nil {
			fnDef.Registry = modReg
			if tv.Parent.Equal(native.TFnDef) {
				return native.NewFnDef(fnDef)
			}
			return native.NewFunction(fnDef)
		}
		return tv
	}
	if val, ok := modReg.Defs.Top(name); ok {
		if fnDef, ok := val.Data.(native.FnDefInfo); ok && fnDef.Registry == nil {
			fnDef.Registry = modReg
			if val.Parent.Equal(native.TFnDef) {
				return native.NewFnDef(fnDef)
			}
			return native.NewFunction(fnDef)
		}
		return val
	}
	return v
}

// activeRun returns the testRun associated with the parent registry,
// lazily creating it on first access.
func activeRun(parent *native.Registry) *testRun {
	if run, ok, _ := eng.Cap[*testRun](parent, capTestRun); ok && run != nil {
		return run
	}
	run := &testRun{}
	_ = parent.Capabilities.Set(capTestRun, run)
	return run
}

// testNatives builds the Go-implemented imperative test API. Words
// are registered into the module sub-registry; their handlers reach
// the active testRun via the captured parent registry.
func testNatives(parent *native.Registry) []native.NativeFunc {
	return []native.NativeFunc{
		// describe "name" [body] — push name onto the path, run body,
		// pop. Body errors abort the describe but leave already-
		// recorded results in place.
		{
			Name: "test-describe",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TString, native.TList},
				NoEvalArgs: map[int]bool{1: true},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					name, _ := args[0].AsConcreteString()
					body, err := native.RequireConcreteList(args[1], "test.describe")
					if err != nil {
						return nil, err
					}
					run := activeRun(parent)
					run.mu.Lock()
					run.path = append(run.path, name)
					run.mu.Unlock()
					_, runErr := native.New(r).Run(body.Slice())
					run.mu.Lock()
					run.path = run.path[:len(run.path)-1]
					run.mu.Unlock()
					if runErr != nil {
						return nil, runErr
					}
					return nil, nil
				},
				Returns: []*native.Type{}, BarrierPos: -1,
			}},
		},
		// test "name" [body] — run body, record a TestResult. Catches
		// assertion errors so other tests continue.
		{
			Name: "test-test",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TString, native.TList},
				NoEvalArgs: map[int]bool{1: true},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					name, _ := args[0].AsConcreteString()
					body, err := native.RequireConcreteList(args[1], "test.test")
					if err != nil {
						return nil, err
					}
					run := activeRun(parent)
					run.runCase(r, name, body.Slice())
					return nil, nil
				},
				Returns: []*native.Type{}, BarrierPos: -1,
			}},
		},
		// test.results — return the accumulated TestResult Table.
		{
			Name: "test-results",
			Signatures: []native.NativeSig{{
				Args: []*native.Type{},
				Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{activeRun(parent).asTable()}, nil
				},
				Returns: []*native.Type{native.TList}, BarrierPos: -1,
			}},
		},
		// test.reset — clear the active TestRun.
		{
			Name: "test-reset",
			Signatures: []native.NativeSig{{
				Args: []*native.Type{},
				Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					run := activeRun(parent)
					run.mu.Lock()
					run.results = nil
					run.failures = 0
					run.path = nil
					run.mu.Unlock()
					return nil, nil
				},
				Returns: []*native.Type{}, BarrierPos: -1,
			}},
		},
		// test.summary — return a Record with pass/fail/total counts.
		{
			Name: "test-summary",
			Signatures: []native.NativeSig{{
				Args: []*native.Type{},
				Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{activeRun(parent).summary()}, nil
				},
				Returns: []*native.Type{native.TMap}, BarrierPos: -1,
			}},
		},
		// test.fail-count — return the failure count as an integer.
		{
			Name: "test-fail-count",
			Signatures: []native.NativeSig{{
				Args: []*native.Type{},
				Handler: func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					run := activeRun(parent)
					run.mu.Lock()
					n := run.failures
					run.mu.Unlock()
					return []native.Value{native.NewInteger(int64(n))}, nil
				},
				Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
			}},
		},

		// --- assertions ---
		// All assertion words raise an AQL error with code
		// "assertion_failure" when they fail. The enclosing test
		// handler catches the error and records the case as failed.
		{
			Name: "assert-equal",
			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TAny, native.TAny},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					// args[0] is the forward / first arg (expected),
					// args[1] is the second (actual). Print order:
					// "expected X, got Y".
					expected, actual := args[0], args[1]
					if !native.ValuesEqual(expected, actual) {
						return nil, r.AqlError("assertion_failure",
							fmt.Sprintf("assert.equal: expected %s, got %s",
								native.FormatForPrint(expected),
								native.FormatForPrint(actual)),
							"assert.equal")
					}
					return nil, nil
				},
				Returns: []*native.Type{}, BarrierPos: -1,
			}},
		},
		{
			Name: "assert-not-equal",
			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TAny, native.TAny},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					if native.ValuesEqual(args[0], args[1]) {
						return nil, r.AqlError("assertion_failure",
							fmt.Sprintf("assert.not-equal: both sides equal %s",
								native.FormatForPrint(args[0])),
							"assert.not-equal")
					}
					return nil, nil
				},
				Returns: []*native.Type{}, BarrierPos: -1,
			}},
		},
		{
			Name: "assert-ok",
			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TAny},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					if !isTruthy(args[0]) {
						return nil, r.AqlError("assertion_failure",
							fmt.Sprintf("assert.ok: value is falsy: %s", native.FormatForPrint(args[0])),
							"assert.ok")
					}
					return nil, nil
				},
				Returns: []*native.Type{}, BarrierPos: -1,
			}},
		},
		{
			Name: "assert-throws",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TList},
				NoEvalArgs: map[int]bool{0: true},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					body, err := native.RequireConcreteList(args[0], "assert.throws")
					if err != nil {
						return nil, err
					}
					_, runErr := native.New(r).Run(body.Slice())
					if runErr == nil {
						return nil, r.AqlError("assertion_failure",
							"assert.throws: body did not throw",
							"assert.throws")
					}
					return nil, nil
				},
				Returns: []*native.Type{}, BarrierPos: -1,
			}},
		},
		{
			Name: "assert-match",
			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TString, native.TString},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					sub, _ := args[0].AsConcreteString()
					s, _ := args[1].AsConcreteString()
					if !strings.Contains(s, sub) {
						return nil, r.AqlError("assertion_failure",
							fmt.Sprintf("assert.match: %q does not contain %q", s, sub),
							"assert.match")
					}
					return nil, nil
				},
				Returns: []*native.Type{}, BarrierPos: -1,
			}},
		},
		// --- spec runner (Go side) ---
		// test.invoke subject-atom inputs-list — call the subject in
		// the parent registry by pushing inputs as tokens and
		// dispatching the word in a sub-engine. Returns the top-of-
		// stack result (or Error value). Runs against `parent` (the
		// caller's registry) — the AQL spec runner lives in the test
		// module's sub-registry, but the subject under test is defined
		// in the caller's scope.
		{
			Name: "test-invoke",
			Signatures: []native.NativeSig{
				{
					Args: []*native.Type{native.TAtom, native.TList},
					Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						name, _ := args[0].AsConcreteAtom()
						return invokeSubject(parent, name, args[1])
					},
					Returns: []*native.Type{native.TAny}, BarrierPos: -1,
				},
				{
					Args: []*native.Type{native.TString, native.TList},
					Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						name, _ := args[0].AsConcreteString()
						return invokeSubject(parent, name, args[1])
					},
					Returns: []*native.Type{native.TAny}, BarrierPos: -1,
				},
			},
		},
		// test.record name path ok expected actual error duration-ms
		//   — append a TestResult to the active TestRun. Used by the
		//   AQL spec runner to assemble results uniformly with the
		//   imperative API.
		{
			Name: "test-record",
			Signatures: []native.NativeSig{{
				Args: []*native.Type{
					native.TString, native.TList, native.TBoolean,
					native.TAny, native.TAny, native.TAny, native.TInteger,
				},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					name, _ := args[0].AsConcreteString()
					pathList, _ := native.AsList(args[1])
					ok, _ := args[2].AsConcreteBoolean()
					duration, _ := args[6].AsConcreteInteger()
					run := activeRun(parent)
					run.mu.Lock()
					defer run.mu.Unlock()
					path := make([]string, 0, pathList.Len())
					for _, p := range pathList.Slice() {
						s, _ := native.AsString(p)
						path = append(path, s)
					}
					run.results = append(run.results, makeResult(name, path, ok, args[3], args[4], args[5], time.Duration(duration)*time.Millisecond))
					if !ok {
						run.failures++
					}
					return nil, nil
				},
				Returns: []*native.Type{}, BarrierPos: -1,
			}},
		},
		// test.prop name gen property → PropertySpec map.
		//   — constructs a PropertySpec with default runs=100, seed=1,
		//   max-shrinks=200. Implemented in Go (not as an AQL fn)
		//   because gen/property are List bodies that would otherwise
		//   be auto-evaluated during fn-param binding; this native
		//   uses NoEvalArgs + explicit Quoted=true to preserve the
		//   bodies intact for the Stage-5 reducer / Stage-3 runner.
		{
			Name: "test-prop",
			Signatures: []native.NativeSig{{
				Args:       []*native.Type{native.TString, native.TList, native.TList},
				NoEvalArgs: map[int]bool{1: true, 2: true},
				Returns:    []*native.Type{native.TMap},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					name, _ := args[0].AsConcreteString()
					gen := args[1]
					prop := args[2]
					// Mark bodies Quoted so subsequent consumers
					// (e.g. run-property's `get` retrieving them
					// from the map) don't auto-evaluate them.
					gen.Quoted = true
					gen.Eval = false
					prop.Quoted = true
					prop.Eval = false
					m := native.NewOrderedMap()
					m.Set("name", native.NewString(name))
					m.Set("gen", gen)
					m.Set("property", prop)
					m.Set("runs", native.NewInteger(100))
					m.Set("seed", native.NewInteger(1))
					m.Set("max-shrinks", native.NewInteger(200))
					return []native.Value{native.NewMap(m)}, nil
				},
			}},
		},
		// test.check-prop name gen property runs seed max-shrinks
		//   — property-based test driver. Runs the generator body
		//   `runs` times, each iteration with a fresh seeded rand
		//   instance bound as `r`. The property body is called with
		//   the generated value on the stack; it must return Boolean.
		//   On the first false return, records a failure with the
		//   generated input. Returns a PropertyResult Map and also
		//   appends it to the active testRun.
		//
		// The `max-shrinks` arg is reserved for the Stage-5 reducer
		// (PBT-PLAN.0.md) — Stage 3 ignores it and reports the raw
		// failing input verbatim.
		{
			Name: "test-check-prop",
			Signatures: []native.NativeSig{{
				Args: []*native.Type{
					native.TString,  // name
					native.TList,    // gen body (quoted)
					native.TList,    // property body (quoted)
					native.TInteger, // runs
					native.TInteger, // seed
					native.TInteger, // max-shrinks (Stage 5; ignored here)
				},
				NoEvalArgs: map[int]bool{1: true, 2: true},
				Returns:    []*native.Type{native.TMap},
				BarrierPos: -1,
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return runCheckProp(parent, args)
				},
			}},
		},
	}
}

// runCheckProp is the PBT inner loop. Extracted for readability —
// the check-prop native's handler delegates here.
func runCheckProp(parent *native.Registry, args []native.Value) ([]native.Value, error) {
	name, _ := args[0].AsConcreteString()
	genList, err := native.RequireConcreteList(args[1], "test.check-prop gen")
	if err != nil {
		return nil, err
	}
	propList, err := native.RequireConcreteList(args[2], "test.check-prop property")
	if err != nil {
		return nil, err
	}
	runs, _ := args[3].AsConcreteInteger()
	seed, _ := args[4].AsConcreteInteger()
	_ = args[5] // max-shrinks: reserved for Stage 5.

	genBody := genList.Slice()
	propBody := propList.Slice()

	var (
		failed       bool
		actualRuns   int64
		failingInput = native.NewNone()
		failingError = native.NewNone()
	)

	for i := int64(0); i < runs; i++ {
		actualRuns++

		// Build the per-iteration seeded rand instance. Each
		// iteration uses (seed + i) so failures replay with a
		// known-good sub-seed.
		randMap, err := BuildSeededRandInstance(seed + i)
		if err != nil {
			failed = true
			failingError = native.NewError(err)
			break
		}

		// Run the generator body in a CallAQL frame where `r` is
		// bound to the iteration's rand instance. Body must leave
		// exactly one value on the stack — the generated input.
		genSig := native.FnSig{
			Params:     []native.FnParam{{Name: "r", Type: native.TMap}},
			Returns:    []*native.Type{native.TAny},
			Body:       append([]native.Value(nil), genBody...),
			BarrierPos: -1,
		}
		genResults, err := parent.CallAQL(&genSig, []native.Value{native.NewMap(randMap)}, nil)
		if err != nil {
			failed = true
			failingError = native.NewError(err)
			break
		}
		if len(genResults) == 0 {
			failed = true
			failingError = native.NewError(parent.AqlError("check_prop_error",
				"generator produced no value", "test.check-prop"))
			break
		}
		input := genResults[len(genResults)-1]

		// Run the property body with `input` on the stack. Body
		// must leave a Boolean. Anything else is a failure.
		propTokens := append([]native.Value{input}, propBody...)
		propResults, err := native.New(parent).Run(propTokens)
		if err != nil {
			failed = true
			failingInput = input
			failingError = native.NewError(err)
			break
		}
		if len(propResults) == 0 {
			failed = true
			failingInput = input
			failingError = native.NewError(parent.AqlError("check_prop_error",
				"property produced no value", "test.check-prop"))
			break
		}
		propTop := propResults[len(propResults)-1]
		propBool, err := propTop.AsConcreteBoolean()
		if err != nil {
			failed = true
			failingInput = input
			failingError = native.NewError(parent.AqlError("check_prop_error",
				fmt.Sprintf("property returned non-Boolean (%s)", propTop.Parent.String()),
				"test.check-prop"))
			break
		}
		if !propBool {
			failed = true
			failingInput = input
			break
		}
	}

	// Build the PropertyResult map.
	result := native.NewOrderedMap()
	result.Set("name", native.NewString(name))
	result.Set("ok", native.NewBoolean(!failed))
	result.Set("runs", native.NewInteger(actualRuns))
	result.Set("failing-input", failingInput)
	// Shrunk fields reserved for Stage 5. Today they mirror the
	// failing input verbatim so consumers see a value.
	result.Set("shrunk-input", failingInput)
	result.Set("shrunk-source", native.NewString(""))
	result.Set("shrunk-cost", native.NewInteger(0))
	result.Set("error", failingError)

	// Append to the active test run so test.results and
	// test.summary pick this up alongside table-driven tests.
	resultVal := native.NewMap(result)
	run := activeRun(parent)
	run.mu.Lock()
	run.results = append(run.results, resultVal)
	if failed {
		run.failures++
	}
	run.mu.Unlock()

	return []native.Value{resultVal}, nil
}

// runCase executes one test body, catching errors and recording a
// TestResult under the current describe path.
func (run *testRun) runCase(r *native.Registry, name string, body []native.Value) {
	run.mu.Lock()
	pathCopy := append([]string(nil), run.path...)
	run.mu.Unlock()

	start := time.Now()
	_, err := native.New(r).Run(body)
	elapsed := time.Since(start)

	ok := err == nil
	var errVal native.Value
	if err != nil {
		errVal = native.NewError(err)
	} else {
		errVal = native.NewNone()
	}
	result := makeResult(name, pathCopy, ok, native.NewNone(), native.NewNone(), errVal, elapsed)

	run.mu.Lock()
	run.results = append(run.results, result)
	if !ok {
		run.failures++
	}
	run.mu.Unlock()
}

// makeResult builds a TestResult Map value matching the schema declared
// in the AQL preamble (testAQLPreamble).
func makeResult(name string, path []string, ok bool, expected, actual, errVal native.Value, elapsed time.Duration) native.Value {
	pathVals := make([]native.Value, len(path))
	for i, p := range path {
		pathVals[i] = native.NewString(p)
	}
	om := native.NewOrderedMap()
	om.Set("name", native.NewString(name))
	om.Set("path", native.NewList(pathVals))
	om.Set("ok", native.NewBoolean(ok))
	om.Set("expected", expected)
	om.Set("actual", actual)
	om.Set("error", errVal)
	om.Set("duration-ms", native.NewInteger(elapsed.Milliseconds()))
	return native.NewMap(om)
}

// asTable wraps the accumulated results as a TableData value so the
// caller can pipe them through `report.table`.
func (run *testRun) asTable() native.Value {
	run.mu.Lock()
	defer run.mu.Unlock()
	rec := native.NewOrderedMap()
	rec.Set("name", native.NewTypeLiteral(native.TString))
	rec.Set("path", native.NewTypeLiteral(native.TList))
	rec.Set("ok", native.NewTypeLiteral(native.TBoolean))
	rec.Set("expected", native.NewTypeLiteral(native.TAny))
	rec.Set("actual", native.NewTypeLiteral(native.TAny))
	rec.Set("error", native.NewTypeLiteral(native.TAny))
	rec.Set("duration-ms", native.NewTypeLiteral(native.TInteger))
	rows := make([]native.Value, len(run.results))
	copy(rows, run.results)
	return native.NewValueRaw(native.TList, native.TableData{
		Record: native.RecordTypeInfo{Fields: rec},
		Rows:   rows,
	})
}

// summary builds a {total, passed, failed} Map for quick reporting.
func (run *testRun) summary() native.Value {
	run.mu.Lock()
	defer run.mu.Unlock()
	total := len(run.results)
	failed := run.failures
	om := native.NewOrderedMap()
	om.Set("total", native.NewInteger(int64(total)))
	om.Set("passed", native.NewInteger(int64(total-failed)))
	om.Set("failed", native.NewInteger(int64(failed)))
	return native.NewMap(om)
}

// invokeSubject runs a subject word against an input list in the
// parent registry. Shared by the Atom and String overloads of
// test-invoke.
func invokeSubject(parent *native.Registry, name string, inputArg native.Value) ([]native.Value, error) {
	inputs, err := native.RequireConcreteList(inputArg, "test.invoke")
	if err != nil {
		return nil, err
	}
	tokens := append([]native.Value(nil), inputs.Slice()...)
	tokens = append(tokens, dottedWordTokens(name)...)
	sub := native.New(parent)
	stack, runErr := sub.Run(tokens)
	if runErr != nil {
		return []native.Value{native.NewError(runErr)}, nil
	}
	if len(stack) == 0 {
		return []native.Value{native.NewNone()}, nil
	}
	return []native.Value{stack[len(stack)-1]}, nil
}

// dottedWordTokens returns the token sequence the engine would
// produce for a dotted reference. A plain "foo" lexes to [Word(foo)];
// "decision.eval-cond" lexes to [Word(decision), Word(get),
// Atom(eval-cond)]. test.invoke uses this so a spec can name its
// subject as either `eval-cond/q` (when the user has imported the
// module's words flat) or `decision.eval-cond/q` (the more common
// form, when the user has the bare module import).
func dottedWordTokens(name string) []native.Value {
	parts := strings.Split(name, ".")
	if len(parts) == 1 {
		return []native.Value{native.NewWord(name)}
	}
	out := make([]native.Value, 0, 1+2*(len(parts)-1))
	out = append(out, native.NewWord(parts[0]))
	for _, p := range parts[1:] {
		out = append(out, native.NewWord("get"), native.NewAtom(p))
	}
	return out
}

// isTruthy mirrors the AQL convention used by `if` / `and` / `or`:
// false, none and the None type literal are falsy; everything else is
// truthy. This keeps `assert.ok` aligned with the language's other
// boolean coercion sites without introducing a new rule.
func isTruthy(v native.Value) bool {
	if native.IsNone(v) {
		return false
	}
	if v.Parent.Matches(native.TBoolean) {
		b, _ := native.AsBoolean(v)
		return b
	}
	if v.Data == nil {
		return false
	}
	return true
}

// testAQLPreamble defines canonical record types and the pure-AQL
// spec runner. It runs inside the test module's sub-registry after
// the Go natives are registered, so the `export` map references both
// Go words (test-test, test-describe, ...) and AQL types (TestCase,
// TestSet, TestSpec, TestResult).
//
// Naming convention: Go words use kebab prefixes (test-X) to avoid
// colliding with user-facing names; the export map renames them to
// the dotted form (test.test, test.describe, assert.equal, ...).
const testAQLPreamble = `

# ============================================================
# aql:test — Record / Table types
# ============================================================

# A single test case in a declarative spec.
# - name:    label printed in reports
# - in:      list of inputs pushed in order onto the subject's stack
# - out:     expected top-of-stack result after the subject runs
def TestCase refine Record [name:String in:List out:Any]

# A set of test cases — a Table over TestCase.
def TestSet refine Table (refine Record [name:String in:List out:Any])

# A whole spec: a named group with a subject (atom or dotted-string
# name referring to a word resolvable in the def stack at run time)
# and either inline cases or sub-specs (or both).
# - subject:  Atom or String naming the word under test. Strings
#             support dotted names like "decision.eval-cond" so a
#             spec can target a module export without first flat-
#             importing the word.
# - cases:    inline TestSet (may be empty)
# - subs:     list of sub-specs (may be empty)
def TestSpec refine Record [name:String subject:Any cases:List subs:List]

# ============================================================
# Property-based testing (PBT) — Stage 3
# ============================================================
# A PropertySpec describes a property to be checked against
# randomly-generated inputs. The framework runs gen runs
# times, each time with a fresh seeded rand instance bound as
# r inside the gen body; the resulting value is then fed to
# property which must return a Boolean. False or an error in
# any iteration is a failure.
#
# - name:         label for the report.
# - gen:          quoted code body that produces ONE value at
#                 stack top. Receives the iteration's rand
#                 instance via the bound name r.
# - property:     quoted code body that takes the generated
#                 value on the stack and leaves a Boolean.
# - runs:         number of random iterations (default 100).
# - seed:         base PRNG seed; iteration k uses seed+k for
#                 independent replay (default 1).
# - max-shrinks:  cap on the Stage-5 reducer's search depth
#                 (default 200; ignored before Stage 5).
def PropertySpec refine Record [
  name:String
  gen:List
  property:List
  runs:Integer
  seed:Integer
  max-shrinks:Integer
]

# Per-property outcome. The shrunk-* fields are populated by the
# Stage-5 reducer; Stage 3 mirrors the raw failing input there.
def PropertyResult refine Record [
  name:String
  ok:Boolean
  runs:Integer
  failing-input:Any
  shrunk-input:Any
  shrunk-source:String
  shrunk-cost:Integer
  error:Any
]

# Per-case outcome recorded by the runner.
def TestResult refine Record [
  name:String
  path:List
  ok:Boolean
  expected:Any
  actual:Any
  error:Any
  duration-ms:Integer
]

# ============================================================
# Helpers to construct specs declaratively
# ============================================================

def case fn [[out:Any in:List name:String] [Map] [
  make TestCase {name:name in:in out:out}
]]

def spec fn [[cases:List subject:Any name:String] [Map] [
  make TestSpec {name:name subject:subject cases:cases subs:[]}
]]

def spec-with-subs fn [[subs:List cases:List subject:Any name:String] [Map] [
  make TestSpec {name:name subject:subject cases:cases subs:subs}
]]

# prop is a Go native constructor — see test-prop in the natives
# table. The bodies are NoEvalArgs-protected at the native boundary
# so list literals like [0 100 r.int] survive intact.

# run-property destructures a PropertySpec map and dispatches the
# Go-side check-prop driver. Returns the PropertyResult map.
#
# The gen/property fields are stored Quoted in the map (set by
# test.prop), so a plain map.get retrieval preserves them as data
# rather than triggering auto-eval as they cross fn boundaries.
#
# Uses FORWARD form for the test-check-prop call so each arg fills
# the corresponding sig position directly (sig[0]=String, sig[1..2]
# =List, sig[3..5]=Integer).
def run-property fn [[| p:Map] [Map] [
  test-check-prop
    (p get "name")
    (p get "gen")
    (p get "property")
    (p get "runs")
    (p get "seed")
    (p get "max-shrinks")
]]

# ============================================================
# Pure-AQL spec runner
# ============================================================
# run-spec invokes each case's subject with the case inputs, compares
# the result to the case's "out" via deep equality, and records the
# outcome through test-record (Go). Sub-specs run recursively under a
# describe scope so their results inherit the parent spec name in the
# path column.

def run-case fn [[| subject:Scalar c:Map] [] [
  def in quote (c get "in")
  def expected (c get "out")
  def actual (in subject test-invoke)
  def matched (expected actual deq)
  0 None actual expected matched [] (c get "name") test-record
]]

def run-cases fn [[| subject:Scalar cases:List] [] [
  for (cases length) [
    def _i i
    def c (cases _i get)
    c subject run-case
  ] end
]]

def run-spec fn [[| s:Map] [] [
  [
    def subject (s get "subject")
    def cases quote (s get "cases")
    def subs quote (s get "subs")
    cases subject run-cases
    for (subs length) [
      def _i i
      def sub (subs _i get)
      sub run-spec
    ] end
  ] (s get "name") test-describe
]]

# ============================================================
# Exports
# ============================================================

export "test" {
  # types
  TestCase:        TestCase
  TestSet:         TestSet
  TestSpec:        TestSpec
  TestResult:      TestResult
  PropertySpec:    PropertySpec
  PropertyResult:  PropertyResult

  # spec constructors
  case:           case
  spec:           spec
  spec-with-subs: spec-with-subs
  prop:           test-prop

  # imperative API (Go)
  describe:    test-describe
  test:        test-test
  it:          test-test
  check-prop:  test-check-prop

  # accumulated results
  results:    test-results
  summary:    test-summary
  reset:      test-reset
  fail-count: test-fail-count

  # spec runner
  run-spec:     run-spec
  run-property: run-property
  invoke:       test-invoke
}

export "assert" {
  equal:      assert-equal
  not-equal:  assert-not-equal
  ok:         assert-ok
  throws:     assert-throws
  match:      assert-match
}

`
