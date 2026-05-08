package engine

import (
	"fmt"
)

// defName extracts a word name from a Value that is either a word or a string.
func defName(v Value) string {
	if v.IsWord() {
		_as0, _ := v.AsWord()
		return _as0.Name
	}
	_as1, _ := v.AsString()
	return _as1
}

// defStackOnly returns true if the name word carries the /s modifier,
// indicating the defined word should be stack-only (not forward precedence).
func defStackOnly(v Value) bool {
	if v.IsWord() {
		_as2, _ := v.AsWord()
		return _as2.ForceStack
	}
	return false
}

// RegisterDef registers the "def" word for defining new words.
//
// def creates literal substitutions: the body replaces the word during
// evaluation. If the body is a list, its elements are spliced into the
// stack. Otherwise the single value is pushed.
//
// Three signatures, sharing a single handler each:
//
//	Args:[TString, TAny]       – def "name" body
//	Args:[TAtom/q, TAny]       – def name body  (word captured as atom via /q)
//	Args:[TMap, TAny]          – def name:Type body  (typed binding)
//
// The /q modifier on the Atom position causes Word values to be treated as
// Atoms for matching, and captured without evaluation during forward
// collection. Forward precedence rules handle all orderings (forward,
// infix, postfix) without separate infix signatures.
//
// The TMap form picks up the surface syntax `def name:Type body`. At the
// top level, jsonic parses `name:Type` as a single-pair map; the handler
// extracts the only key as the name, the only value as a type
// constraint, and unifies the body with the constraint before
// installing. Multi-key maps and non-type values are rejected.
func RegisterDef(r *Registry) {
	defHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		name := defName(args[0])
		stackOnly := defStackOnly(args[0])
		body := args[1]
		if IsCapitalisedName(name) {
			return nil, fmt.Errorf("def %s: def names must not start with a capital letter (capitalised names are reserved for types)", name)
		}
		// Refuse a def whose name is already a registered TYPE — type
		// and def share the same Word namespace so a single name
		// must mean exactly one thing.
		if r.HasType(name) {
			return nil, fmt.Errorf("def %s: name clash — already a type", name)
		}
		InstallDef(r, name, body, stackOnly)
		// Record installation for unused-def analysis. The arg's
		// Pos points at the name token.
		r.RecordCheckDef(name, args[0].Pos)
		return nil, nil
	}

	defTypedHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		nameMap := args[0].AsMap()
		if nameMap == nil || nameMap.Len() == 0 {
			return nil, fmt.Errorf("def: typed-name map must have exactly one key, got empty/non-concrete map")
		}
		if nameMap.Len() != 1 {
			return nil, fmt.Errorf("def: typed-name map must have exactly one key, got %d", nameMap.Len())
		}
		name := nameMap.Keys()[0]
		if IsCapitalisedName(name) {
			return nil, fmt.Errorf("def %s: def names must not start with a capital letter (capitalised names are reserved for types)", name)
		}
		if r.HasType(name) {
			return nil, fmt.Errorf("def %s: name clash — already a type", name)
		}
		constraint, _ := nameMap.Get(name)
		// NoEvalMapArgs suppresses the generic autoEvalMap pipeline for
		// this slot, so a Word at the type position arrives raw.
		// Resolve named user-defined types via r.Types (the dedicated
		// type registry) first; fall back to DefStacks so legacy
		// type-definition kinds that still pass through InstallDef
		// (records, ObjectType, DepScalar, …) keep working until the
		// full migration completes. Capture the source name when the
		// constraint comes from a registered type — the error path
		// uses it so messages say "type Bbd" rather than just printing
		// the resolved value form.
		var typeName string
		constraint, typeName, _ = r.ResolveTypedNameValue(constraint)
		if !IsTypeBody(constraint) {
			return nil, fmt.Errorf("def %s: type annotation must be a type value, got %s", name, constraint.String())
		}
		// describeType returns the user-facing label for the
		// constraint: the registered name when one is known, the
		// rendered value form otherwise.
		describeType := func() string {
			if typeName != "" {
				return typeName
			}
			return constraint.String()
		}
		// When the constraint is a function-shape type and the body is
		// a quoted atom naming a defined function, resolve to the
		// function value before unifying. This lets the user write
		// `def m:Mapper (quote double)` to bind m to the function
		// double — quote's normal output is an Atom, which would
		// never unify with a FnUndef constraint otherwise.
		body := args[1]
		if constraint.VType.Equal(TFnUndef) && body.IsAtom() {
			atomName, _ := body.AsAtom()
			if top, ok := r.TopOfDefStack(atomName); ok {
				if top.VType.Equal(TFnDef) || top.VType.Equal(TFunction) {
					body = top
				}
			}
		}
		// Predicate type: the constraint is a fn whose body unifies the
		// candidate against the type. The fn returns None on failure
		// or the unified value on success — typically the candidate
		// itself, but a coercive predicate may return a transformed
		// value. The def installs with the *returned* value, not the
		// candidate, so a predicate like `[x upper]` actually rebinds
		// to the transformed shape.
		if constraint.VType.Equal(TFnDef) || constraint.VType.Equal(TFunction) {
			out, matched, err := r.RunPredicate(constraint, body)
			if err != nil {
				return nil, fmt.Errorf("def %s: predicate type %s: %w", name, describeType(), err)
			}
			if !matched {
				return nil, fmt.Errorf("def %s: value %s does not satisfy predicate type %s",
					name, body.String(), describeType())
			}
			InstallDef(r, name, out)
			r.RecordCheckDef(name, args[0].Pos)
			return nil, nil
		}
		// CheckMode + DepScalar: under static analysis the body is
		// frequently a carrier whose payload Unify's DepScalar
		// branch can't compare against the bound — every typed
		// binding would then error. For DepScalar constraints we
		// answer the type-level question symbolically: if the
		// body's VType matches the dependent's base type, accept
		// the binding. The per-value test (does 5 lie in [10, ∞)?)
		// stays runtime-only; the analyser only verifies the
		// shape.
		if r.IsCheckMode() && constraint.IsDepScalar() {
			leaf := DependentLeafFromType(constraint.VType)
			if base, ok := DependentLeafBaseType(leaf); ok && body.VType.Matches(base) {
				InstallDef(r, name, body)
				r.RecordCheckDef(name, args[0].Pos)
				return nil, nil
			}
		}
		unified, ok := Unify(body, constraint)
		if !ok {
			// In check mode, surface the type mismatch as a diagnostic
			// AND install a constraint-typed carrier so downstream code
			// that uses `name` doesn't cascade with "undefined word"
			// noise. The runtime path still aborts — only check mode
			// keeps flowing past the mismatch (§6.3).
			if r.IsCheckMode() {
				r.AddCheckDiagnostic(CheckDiagnostic{
					Code: "type_error",
					Detail: fmt.Sprintf("def %s: value %s does not unify with declared type %s",
						name, body.String(), describeType()),
					Word: name,
					Row:  args[0].Pos.Row,
					Col:  args[0].Pos.Col,
				})
				InstallDef(r, name, NewCarrier(constraint.VType))
				r.RecordCheckDef(name, args[0].Pos)
				return nil, nil
			}
			return nil, fmt.Errorf("def %s: value %s does not unify with declared type %s",
				name, body.String(), describeType())
		}
		InstallDef(r, name, unified)
		r.RecordCheckDef(name, args[0].Pos)
		return nil, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "def",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				// Typed-name binding: def name:Type body. Sorts first
				// because TMap is more specific than TString / TAtom
				// at the same depth (higher inherent score).
				// NoEvalMapArgs[0]=true keeps the type-name map's value
				// raw so the handler can resolve it through DefStacks
				// itself — important for fn-as-type names that double
				// as registered callables.
				Args:           []Type{TMap, TAny},
				NoEvalArgs:     map[int]bool{1: true},
				NoEvalMapArgs:  map[int]bool{0: true},
				Handler:        defTypedHandler,
				Returns:        []Type{},
				RunInCheckMode: true,
			},
			{
				Args:           []Type{TString, TAny},
				NoEvalArgs:     map[int]bool{1: true},
				Handler:        defHandler,
				Returns:        []Type{},
				RunInCheckMode: true,
			},
			{
				Args:           []Type{TAtom, TAny},
				QuoteArgs:      map[int]bool{0: true},
				NoEvalArgs:     map[int]bool{1: true},
				Handler:        defHandler,
				Returns:        []Type{},
				RunInCheckMode: true,
			},
		},
	})
}

// InstallDef: re-exported from aqleng via aliases.go

// FnDefsOverlap: re-exported from aqleng via aliases.go

// UninstallDef: re-exported from aqleng via aliases.go
