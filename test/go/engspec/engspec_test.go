// Spec-runner test for the engine kernel — runs the shared corpus at
// aql/eng/spec/*.tsv (sibling of eng/go/ and eng/ts/, so Go and TypeScript
// ports run the same .tsv files). Each row is parsed with the AQL parser
// (eng/parser) and run against a fresh eng.Registry pre-populated with
// eng.RegisterCoreWords plus a fixed set of spec-runner test fixtures
// (q-suffixed). No production native words (add, upper, …) are installed
// — the q-fixtures cover dispatch / value / type-lattice ground in
// spec-stable minimal forms.
//
// The "q" suffix on most fixtures marks them as SPEC-RUNNER FIXTURES,
// distinct from production AQL words of the same root name. Language-
// fundamental keywords (def, fn, quote, args, type, untype, typeof,
// is, none, end, …) keep their bare names because what's being tested
// IS the keyword itself, not a fixture for it.
//
// This file lives in the test module (not eng/go) so eng/go has no
// dependency on test — the dep arrow points one way: test → eng.
package engspec

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng"
	"github.com/aql-lang/aql/eng/parser"
	"github.com/aql-lang/aql/test/go/specrunner"
)

// specReplayCounter is bumped per call to the `replayq` test fixture so
// each Mark/Move pair gets a unique ID across a spec file.
var specReplayCounter int

// registerSpecWords installs the eng core words plus the spec-runner
// test fixtures on a registry. The fixtures are minimal, single-overload
// variants tailored for spec coverage of the dispatch / value /
// type-lattice core.
func registerSpecWords(r *eng.Registry) {
	eng.RegisterCoreWords(r)

	toFloat := func(v eng.Value) float64 {
		if v.VType.Matches(eng.TInteger) {
			n, _ := eng.AsInteger(v)
			return float64(n)
		}
		f, _ := eng.AsDecimal(v)
		return f
	}
	numericBinary := func(intOp func(a, b int64) int64, floatOp func(a, b float64) float64) eng.Handler {
		return func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
			if args[0].VType.Matches(eng.TInteger) && args[1].VType.Matches(eng.TInteger) {
				a, _ := eng.AsInteger(args[0])
				b, _ := eng.AsInteger(args[1])
				return []eng.Value{eng.NewInteger(intOp(a, b))}, nil
			}
			return []eng.Value{eng.NewDecimal(floatOp(toFloat(args[0]), toFloat(args[1])))}, nil
		}
	}
	numberPair := []*eng.Type{eng.TNumber, eng.TNumber}

	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "addq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args:    numberPair,
			Handler: numericBinary(func(a, b int64) int64 { return b + a }, func(a, b float64) float64 { return b + a }),
			Returns: []*eng.Type{eng.TNumber},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "subq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args:    numberPair,
			Handler: numericBinary(func(a, b int64) int64 { return b - a }, func(a, b float64) float64 { return b - a }),
			Returns: []*eng.Type{eng.TNumber},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "mulq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args:    numberPair,
			Handler: numericBinary(func(a, b int64) int64 { return b * a }, func(a, b float64) float64 { return b * a }),
			Returns: []*eng.Type{eng.TNumber},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "negq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{eng.TNumber}, BarrierPos: 1,
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				if args[0].VType.Matches(eng.TInteger) {
					n, _ := eng.AsInteger(args[0])
					return []eng.Value{eng.NewInteger(-n)}, nil
				}
				f, _ := eng.AsDecimal(args[0])
				return []eng.Value{eng.NewDecimal(-f)}, nil
			},
			Returns: []*eng.Type{eng.TNumber},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "concatq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{eng.TString, eng.TString},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				a, _ := eng.AsString(args[0])
				b, _ := eng.AsString(args[1])
				return []eng.Value{eng.NewString(b + a)}, nil
			},
			Returns: []*eng.Type{eng.TString},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "describeq", ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{
				Args: []*eng.Type{eng.TInteger},
				Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					n, _ := eng.AsInteger(args[0])
					return []eng.Value{eng.NewString("int:" + strconv.FormatInt(n, 10))}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
			{
				Args: []*eng.Type{eng.TString},
				Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					s, _ := eng.AsString(args[0])
					return []eng.Value{eng.NewString("str:" + s)}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "tagq", ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{Args: []*eng.Type{eng.TAny}, Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				return []eng.Value{eng.NewString("any")}, nil
			}, Returns: []*eng.Type{eng.TString}},
			{Args: []*eng.Type{eng.TInteger}, Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				return []eng.Value{eng.NewString("specific")}, nil
			}, Returns: []*eng.Type{eng.TString}},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "factq", ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{
				Args: []*eng.Type{eng.TInteger}, Patterns: map[int]eng.Value{0: eng.NewInteger(0)},
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewInteger(1)}, nil
				},
				Returns: []*eng.Type{eng.TInteger},
			},
			{
				Args: []*eng.Type{eng.TInteger},
				Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					n, _ := eng.AsInteger(args[0])
					return []eng.Value{eng.NewInteger(n)}, nil
				},
				Returns: []*eng.Type{eng.TInteger},
			},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "codeq", ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{
				Args: []*eng.Type{eng.TInteger}, Patterns: map[int]eng.Value{0: eng.NewInteger(99)},
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewString("ninety-nine")}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
			{
				Args: []*eng.Type{eng.TInteger},
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewString("general")}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "routeq", ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{
				Args: []*eng.Type{eng.TString}, Patterns: map[int]eng.Value{0: eng.NewString("admin")},
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewString("matched-admin")}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
			{
				Args: []*eng.Type{eng.TString},
				Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					return []eng.Value{eng.NewString("other")}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "tripq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{eng.TInteger, eng.TInteger, eng.TInteger},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				a, _ := eng.AsInteger(args[0])
				b, _ := eng.AsInteger(args[1])
				c, _ := eng.AsInteger(args[2])
				return []eng.Value{eng.NewString(fmt.Sprintf("%d,%d,%d", a, b, c))}, nil
			},
			Returns: []*eng.Type{eng.TString},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "pairq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args:       []*eng.Type{eng.TInteger, eng.TInteger},
			BarrierPos: 1,
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				a, _ := eng.AsInteger(args[0])
				b, _ := eng.AsInteger(args[1])
				return []eng.Value{eng.NewString(fmt.Sprintf("%d:%d", a, b))}, nil
			},
			Returns: []*eng.Type{eng.TString},
		}},
	})

	// ── Barrier / arity fixtures (for barrier.tsv) ────────────────
	// nilq — a 0-arg word. Exercises 0-arity sigs and the `/0`
	// argCount filter (the fallback-section match path).
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "nilq",
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{},
			Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				return []eng.Value{eng.NewString("nil")}, nil
			},
			Returns: []*eng.Type{eng.TString},
		}},
	})

	// flexq — two overloads of different arity, [Integer] and
	// [Integer, Integer], both forward-eligible (BarrierPos = N). The
	// 1-arg sig is tried first, so a bare `flexq` always picks it; the
	// `/N` argCount modifier (flexq/1, flexq/2) selects the overload
	// explicitly, and `/1f`, `/2s` etc. combine arity selection with a
	// forced forward/stack boundary.
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "flexq", ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{
				Args: []*eng.Type{eng.TInteger},
				Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					a, _ := eng.AsInteger(args[0])
					return []eng.Value{eng.NewString(fmt.Sprintf("one:%d", a))}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
			{
				Args: []*eng.Type{eng.TInteger, eng.TInteger},
				Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
					a, _ := eng.AsInteger(args[0])
					b, _ := eng.AsInteger(args[1])
					return []eng.Value{eng.NewString(fmt.Sprintf("two:%d,%d", a, b))}, nil
				},
				Returns: []*eng.Type{eng.TString},
			},
		},
	})

	// Fixed-arity Integer formatters with an intrinsic barrier at the
	// position named by the numeric suffix (the un-suffixed "main"
	// series — pairq=2/B1, tripq=3/B3, quadq=4/B2, quintq=5/B3,
	// hexq=6/B3, septq=7/B4 — plus tri1q/tri2q and quad1q/quad3q for
	// the off-centre boundaries). Each handler renders its args in
	// signature order, comma-separated, so a row's output reveals
	// exactly which source token bound to which sig position.
	// Combined with /s, /f and /N the rows reach every boundary
	// position 0..N for arities 1..7.
	intArgsFmt := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
		parts := make([]string, len(args))
		for i, a := range args {
			n, _ := eng.AsInteger(a)
			parts[i] = strconv.FormatInt(n, 10)
		}
		return []eng.Value{eng.NewString(strings.Join(parts, ","))}, nil
	}
	intArity := func(name string, n, barrier int) {
		args := make([]*eng.Type, n)
		for i := range args {
			args[i] = eng.TInteger
		}
		r.RegisterNativeFunc(eng.NativeFunc{
			Name: name, ForwardArgs: true,
			Signatures: []eng.NativeSig{{
				Args: args, BarrierPos: barrier,
				Handler: intArgsFmt,
				Returns: []*eng.Type{eng.TString},
			}},
		})
	}
	intArity("tri1q", 3, 1)
	intArity("tri2q", 3, 2)
	intArity("quad1q", 4, 1)
	intArity("quadq", 4, 2)
	intArity("quad3q", 4, 3)
	intArity("quintq", 5, 3)
	intArity("hexq", 6, 3)
	intArity("septq", 7, 4)

	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "lengthq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{eng.TList},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				lst, _ := eng.AsList(args[0])
				return []eng.Value{eng.NewInteger(int64(lst.Len()))}, nil
			},
			Returns: []*eng.Type{eng.TInteger},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "firstq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args: []*eng.Type{eng.TList},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				lst, _ := eng.AsList(args[0])
				if lst.Len() == 0 {
					return []eng.Value{eng.NewNone()}, nil
				}
				return []eng.Value{lst.Get(0)}, nil
			},
			Returns: []*eng.Type{eng.TAny},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "replayq", ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args:       []*eng.Type{eng.TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
				_lst, _ := eng.AsList(args[0])
				body := _lst.Slice()
				specReplayCounter++
				id := fmt.Sprintf("__replayq_%d", specReplayCounter)
				out := make([]eng.Value, 0, len(body)+2)
				out = append(out, eng.NewMark(id, body...))
				out = append(out, body...)
				out = append(out, eng.NewMove(id, "replayq"))
				return out, nil
			},
		}},
	})

	r.Defs.Push("pi", eng.NewInteger(3))
	r.Defs.Push("tau", eng.NewInteger(6))
	r.Defs.Push("greeting", eng.NewString("hello"))

	// break / continue — the production words live in lang
	// (lang/engine/native_control.go); for engspec we register
	// kernel-side stubs that signal Registry.FlowCtrl so the
	// interp.tsv "break outside loop" rows exercise the Run-loop
	// dispatch (which IS kernel territory) without dragging the
	// whole lang word set into the engspec setup.
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "break",
		Signatures: []eng.NativeSig{{
			Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, r *eng.Registry) ([]eng.Value, error) {
				r.FlowCtrl = eng.FlowBreak
				return nil, nil
			},
			Returns: []*eng.Type{},
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "continue",
		Signatures: []eng.NativeSig{{
			Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, r *eng.Registry) ([]eng.Value, error) {
				r.FlowCtrl = eng.FlowContinue
				return nil, nil
			},
			Returns: []*eng.Type{},
		}},
	})

	// not / and / or — production registrations live in
	// lang/engine/native_boolean.go. The eng kernel exposes only the
	// CoerceBoolean primitive; engspec wires it into bare not/and/or
	// names so the existing eng/spec tsvs (forth.tsv, inspect.tsv,
	// types.tsv, …) keep exercising the dispatch path.
	registerEngSpecBoolean(r)
	registerEngSpecTypeOps(r)
	registerEngSpecDo(r)
	registerEngSpecFnSig(r)
	registerEngSpecObjectRecord(r)
}

// registerEngSpecObjectRecord installs `record` and `object` as
// spec-runner fixtures so the eng/spec/record.tsv and
// eng/spec/object.tsv rows can run against the kernel alone. The
// production registrations live in
// lang/engine/native_object_record.go.
func registerEngSpecObjectRecord(r *eng.Registry) {
	recordH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, r *eng.Registry) ([]eng.Value, error) {
		list := args[0]
		if !list.VType.Equal(eng.TList) {
			return nil, fmt.Errorf("record: argument must be a list")
		}
		if list.Data == nil {
			return nil, fmt.Errorf("record: argument must be a concrete list, got type literal")
		}
		elems, _ := eng.AsList(list)
		if elems.Len() == 0 {
			return nil, fmt.Errorf("record: list must have at least one field")
		}
		fields := eng.NewOrderedMap()
		for _, elem := range elems.Slice() {
			if !elem.VType.Equal(eng.TMap) {
				return nil, fmt.Errorf("record: each element must be a pair (map), got %s", elem.String())
			}
			m, err := eng.AsMutableMap(elem)
			if err != nil {
				return nil, fmt.Errorf("record: each element must be a concrete pair, got %s", elem.String())
			}
			for _, key := range m.Keys() {
				val, _ := m.Get(key)
				val = eng.ResolveFieldType(r, val)
				fields.Set(key, val)
			}
		}
		return []eng.Value{eng.NewRecordType(fields)}, nil
	}
	parseObjectFields := func(fieldsMap *eng.OrderedMap, r *eng.Registry) *eng.OrderedMap {
		fields := eng.NewOrderedMap()
		for _, key := range fieldsMap.Keys() {
			val, _ := fieldsMap.Get(key)
			val = eng.ResolveFieldType(r, val)
			fields.Set(key, val)
		}
		return fields
	}
	objectH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, r *eng.Registry) ([]eng.Value, error) {
		fieldsVal := args[0]
		if !fieldsVal.VType.Equal(eng.TMap) {
			return nil, fmt.Errorf("object: argument must be a map of field definitions, got %s", fieldsVal.String())
		}
		m, err := eng.AsMutableMap(fieldsVal)
		if err != nil {
			return nil, fmt.Errorf("object: argument must be a concrete map, got %s", fieldsVal.String())
		}
		fields := parseObjectFields(m, r)
		id := eng.GenerateObjectTypeID()
		info := eng.ObjectTypeInfo{Fields: fields, Parent: nil, ID: id, Name: ""}
		def := r.Types.MintType(id, eng.TObject)
		return []eng.Value{eng.NewObjectType(def, info)}, nil
	}
	objectWithParentH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, r *eng.Registry) ([]eng.Value, error) {
		fieldsVal := args[0]
		parentVal := args[1]
		if !fieldsVal.VType.Equal(eng.TMap) {
			return nil, fmt.Errorf("object: first argument must be a map of field definitions, got %s", fieldsVal.String())
		}
		m, err := eng.AsMutableMap(fieldsVal)
		if err != nil {
			return nil, fmt.Errorf("object: first argument must be a concrete map, got %s", fieldsVal.String())
		}
		if !eng.IsObjectType(parentVal) {
			return nil, fmt.Errorf("object: parent must be an object type, got %s", parentVal.String())
		}
		parentInfo, _ := eng.AsObjectType(parentVal)
		fields := parseObjectFields(m, r)
		parentAllFields := parentInfo.AllFields()
		for _, key := range fields.Keys() {
			childConstraint, _ := fields.Get(key)
			parentConstraint, exists := parentAllFields.Get(key)
			if !exists {
				continue
			}
			if _, ok := eng.Unify(parentConstraint, childConstraint); !ok {
				return nil, fmt.Errorf("object: field %q in child type cannot expand parent type %s (child: %s, parent: %s)",
					key, parentInfo.Name, childConstraint.String(), parentConstraint.String())
			}
		}
		id := eng.GenerateObjectTypeID()
		info := eng.ObjectTypeInfo{Fields: fields, Parent: &parentInfo, ID: id, Name: ""}
		parentDef := parentInfo.Type
		if parentDef == nil {
			parentDef = eng.TObject
		}
		def := r.Types.MintType(id, parentDef)
		return []eng.Value{eng.NewObjectType(def, info)}, nil
	}
	r.RegisterNativeFunc(eng.NativeFunc{
		Name:        "record",
		ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args:           []*eng.Type{eng.TList},
			Handler:        recordH,
			Returns:        []*eng.Type{eng.TRecord},
			RunInCheckMode: true,
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name:        "object",
		ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{
				Args:           []*eng.Type{eng.TMap, eng.TObject},
				Handler:        objectWithParentH,
				Returns:        []*eng.Type{eng.TObjectType},
				RunInCheckMode: true,
			},
			{
				Args:           []*eng.Type{eng.TMap},
				Handler:        objectH,
				Returns:        []*eng.Type{eng.TObjectType},
				RunInCheckMode: true,
			},
		},
	})
}

// registerEngSpecFnSig installs `fnsig` as a spec-runner fixture so
// the eng/spec/types.tsv rows around FnSig type-shape matching can
// run against the kernel alone. The production fnsig registration
// lives in lang/engine/native_definition.go.
func registerEngSpecFnSig(r *eng.Registry) {
	r.RegisterNativeFunc(eng.NativeFunc{
		Name:        "fnsig",
		ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args:       []*eng.Type{eng.TList},
			NoEvalArgs: map[int]bool{0: true},
			Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, reg *eng.Registry) ([]eng.Value, error) {
				if args[0].Data == nil {
					return nil, &eng.AqlError{
						Code:   "fnsig_invalid_spec",
						Detail: "fnsig: argument must be a concrete list",
					}
				}
				lst, _ := eng.AsList(args[0])
				spec := lst.Slice()
				if len(spec) == 0 || len(spec)%2 != 0 {
					return nil, &eng.AqlError{
						Code:   "fnsig_invalid_spec",
						Detail: "fnsig: list length must be a non-zero multiple of 2 (input output pairs); use `fn` for the with-body form",
					}
				}
				info, err := eng.ParseFnUndefSpec(reg, spec)
				if err != nil {
					return nil, err
				}
				return []eng.Value{eng.NewFnUndef(info)}, nil
			},
			Returns: []*eng.Type{eng.TFnUndef},
		}},
	})
}

// registerEngSpecBoolean installs not/and/or as spec-runner fixtures
// using eng.CoerceBoolean. The production words with the same names
// live in lang/engine/native_boolean.go.
func registerEngSpecBoolean(r *eng.Registry) {
	notH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
		return []eng.Value{eng.NewBoolean(!eng.CoerceBoolean(args[0]))}, nil
	}
	andH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
		if !eng.CoerceBoolean(args[1]) {
			return []eng.Value{args[1]}, nil
		}
		return []eng.Value{args[0]}, nil
	}
	orH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
		if eng.CoerceBoolean(args[1]) {
			return []eng.Value{args[1]}, nil
		}
		return []eng.Value{args[0]}, nil
	}
	r.RegisterNativeFunc(eng.NativeFunc{
		Name:        "not",
		ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{Args: []*eng.Type{eng.TBoolean}, Handler: notH, Returns: []*eng.Type{eng.TBoolean}},
			{Args: []*eng.Type{eng.TAny}, Handler: notH, Returns: []*eng.Type{eng.TBoolean}},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name:        "and",
		ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{Args: []*eng.Type{eng.TBoolean, eng.TBoolean}, Handler: andH, Returns: []*eng.Type{eng.TBoolean}},
			{Args: []*eng.Type{eng.TAny, eng.TAny}, Handler: andH, Returns: []*eng.Type{eng.TAny}},
		},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name:        "or",
		ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{Args: []*eng.Type{eng.TBoolean, eng.TBoolean}, BarrierPos: 1, Handler: orH, Returns: []*eng.Type{eng.TBoolean}},
			{Args: []*eng.Type{eng.TAny, eng.TAny}, BarrierPos: 1, Handler: orH, Returns: []*eng.Type{eng.TAny}},
		},
	})
}

// registerEngSpecDo installs the `do` word as a spec-runner fixture.
// The production registration lives in
// lang/engine/native_control.go; engspec ships a minimal version
// that runs a list body or evaluates embedded lists in a map literal
// against a sub-engine — enough surface for the eng/spec/do.tsv rows
// to exercise the kernel's sub-engine semantics.
func registerEngSpecDo(r *eng.Registry) {
	listH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, r *eng.Registry) ([]eng.Value, error) {
		if !eng.IsConcrete(args[0]) {
			return nil, &eng.AqlError{
				Code:   "type_error",
				Detail: "do: argument must be a concrete list, got type literal",
			}
		}
		lst, _ := eng.AsList(args[0])
		sub := eng.New(r)
		input := append([]eng.Value{}, lst.Slice()...)
		result, err := sub.Run(input)
		if err != nil {
			return []eng.Value{eng.NewError(err)}, nil
		}
		return result, nil
	}
	mapH := func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, r *eng.Registry) ([]eng.Value, error) {
		result, err := doEvalMapValue(r, args[0])
		if err != nil {
			return nil, err
		}
		return []eng.Value{result}, nil
	}
	r.RegisterNativeFunc(eng.NativeFunc{
		Name:        "do",
		ForwardArgs: true,
		Signatures: []eng.NativeSig{
			{Args: []*eng.Type{eng.TList}, NoEvalArgs: map[int]bool{0: true}, Handler: listH, Returns: []*eng.Type{eng.TAny}},
			{Args: []*eng.Type{eng.TMap}, Handler: mapH, Returns: []*eng.Type{eng.TAny}},
		},
	})
}

// doEvalMapValue recursively evaluates list values within a map.
// Records, options, table types, typed lists / maps are left
// untouched — only plain concrete lists and maps are walked.
func doEvalMapValue(r *eng.Registry, v eng.Value) (eng.Value, error) {
	if v.VType.Equal(eng.TList) && v.Data != nil && !eng.IsTypedList(v) && !eng.IsTableType(v) {
		lst, _ := eng.AsList(v)
		sub := eng.New(r)
		input := make([]eng.Value, lst.Len())
		for i, e := range lst.Slice() {
			input[i] = doPromoteToWord(r, e)
		}
		results, err := sub.Run(input)
		if err != nil {
			return eng.Value{}, err
		}
		if len(results) == 1 {
			return results[0], nil
		}
		return eng.NewList(results), nil
	}
	if v.VType.Equal(eng.TMap) && v.Data != nil && !eng.IsTypedMap(v) && !eng.IsRecordType(v) && !eng.IsOptionsType(v) {
		m, err := eng.AsMap(v)
		if err != nil || m == nil {
			return v, nil
		}
		out := eng.NewOrderedMap()
		for _, key := range m.Keys() {
			val, _ := m.Get(key)
			evaluated, err := doEvalMapValue(r, val)
			if err != nil {
				return eng.Value{}, err
			}
			out.Set(key, evaluated)
		}
		return eng.NewMap(out), nil
	}
	return v, nil
}

// doPromoteToWord converts a string or atom to a Word when the
// payload names a registered function — so `{op:[1 "add" 2]}` lets
// `do` dispatch "add" as a callable inside the embedded list.
func doPromoteToWord(r *eng.Registry, v eng.Value) eng.Value {
	if v.VType.Matches(eng.TString) || v.VType.Matches(eng.TAtom) {
		name, _ := eng.AsString(v)
		if r.Lookup(name) != nil {
			return eng.NewWord(name)
		}
	}
	return v
}

// registerEngSpecTypeOps installs tor/tand as spec-runner fixtures
// using the eng-exported algorithm handlers. Production registrations
// live in lang/engine/native_type.go.
func registerEngSpecTypeOps(r *eng.Registry) {
	r.RegisterNativeFunc(eng.NativeFunc{
		Name:        "tor",
		ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args:       []*eng.Type{eng.TAny, eng.TAny},
			BarrierPos: 1,
			Handler:    eng.TorHandler,
			ReturnsFn:  eng.TorReturnsFn,
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name:        "tand",
		ForwardArgs: true,
		Signatures: []eng.NativeSig{{
			Args:       []*eng.Type{eng.TAny, eng.TAny},
			BarrierPos: 1,
			Handler:    eng.TandHandler,
			Returns:    []*eng.Type{eng.TAny},
		}},
	})
}

// TestSpec runs aql/eng/spec/*.tsv against the engine kernel — a fresh
// eng.Registry populated with eng.RegisterCoreWords plus the spec-runner
// fixtures registered by registerSpecWords above. The shared TSV
// scaffolding (file walk, row parsing, ERROR handling, value rendering)
// lives in test/go/specrunner.
func TestSpec(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "eng", "spec")
	specrunner.RunDir(t, specDir, func(input string) ([]eng.Value, error) {
		values, err := parser.Parse(input)
		if err != nil {
			return nil, err
		}
		r, err := eng.NewRegistry()
		if err != nil {
			return nil, err
		}
		registerSpecWords(r)
		r.InitRootContext()
		return eng.NewTop(r).Run(values)
	})
}
