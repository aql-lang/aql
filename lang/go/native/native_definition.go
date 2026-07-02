package native

import (
	"fmt"
	"strings"

	"github.com/aql-lang/aql/eng/go"
)

// definitionNatives covers the binding / function-definition words:
// def, undef, var, fn, args, __pa.
//
// Pure helpers used by these handlers (parseFnDef, parseFnParams,
// MatchFnSig, defName, defStackOnly, etc.) live alongside their
// callers in native_definition_fn.go and native_definition_helpers.go.
var definitionNatives = []NativeFunc{
	{
		Name: "def",

		Signatures: []Signature{
			{
				// Typed-name binding: def name:*Type body. Sorts first
				// because TMap is more specific than TString / TAtom
				// at the same depth (higher inherent score).
				// The body is auto-evaluated like any value argument: a
				// list binds like a map (`def xs [1 add 2]` → `[3]`). For a
				// raw / spliced body use `def name word value`.
				Args:          []*Type{TMap, TAny},
				NoEvalMapArgs: map[int]bool{0: true},
				Impl:          Go(defTypedHandler, RunInCheck()),
				Returns:       []*Type{},
				BarrierPos:    -1,
			},
			{
				Args:       []*Type{TString, TAny},
				Impl:       Go(defHandler, RunInCheck()),
				Returns:    []*Type{},
				BarrierPos: -1,
			},
			{
				Args:       []*Type{TAtom, TAny},
				QuoteArgs:  map[int]bool{0: true},
				Impl:       Go(defHandler, RunInCheck()),
				Returns:    []*Type{},
				BarrierPos: -1,
			},
		},
	},
	{
		Name: "undef",

		Signatures: []Signature{
			{
				Args:       []*Type{TString},
				Impl:       Go(undefHandler, RunInCheck()),
				Returns:    []*Type{},
				BarrierPos: -1,
			},
			{
				Args:       []*Type{TAtom},
				QuoteArgs:  map[int]bool{0: true},
				Impl:       Go(undefHandler, RunInCheck()),
				Returns:    []*Type{},
				BarrierPos: -1,
			},
			{
				Args:       []*Type{TString, TFnUndef},
				Impl:       Go(undefFnHandler, RunInCheck()),
				Returns:    []*Type{},
				BarrierPos: -1,
			},
			{
				Args:       []*Type{TAtom, TFnUndef},
				QuoteArgs:  map[int]bool{0: true},
				Impl:       Go(undefFnHandler, RunInCheck()),
				Returns:    []*Type{},
				BarrierPos: -1,
			},
		},
	},
	{
		// __varundef is the cleanup unbind the `var` splice emits — semantically
		// ALWAYS the single-name form (`undef name`). It exists separately from
		// `undef` precisely because `undef` is OVERLOADED with a 2-arg fn-overload
		// form (`undef name fnUndefSpec`): the var splice runs the cleanup with the
		// body's RESIDUAL still on the stack, and in check mode that residual is a
		// dynamic-Any carrier which gradually matches the 2-arg form's TFnUndef
		// slot — so `undef name` mis-dispatched to undefFnHandler and errored
		// ("expected fn undef spec"), leaking the loop binding and refusing the
		// closure. A dedicated 1-arg-only word can never mis-match the residual, so
		// it dispatches identically (1-arg unbind) in check mode and at runtime —
		// the property the compiled `each`/`fold`/… var-body closure needs. Reuses
		// undefHandler so the unbind behaviour is byte-identical to `undef name`.
		Name: "__varundef",
		Signatures: []Signature{
			{
				Args:       []*Type{TString},
				Impl:       Go(undefHandler, RunInCheck()),
				Returns:    []*Type{},
				BarrierPos: -1,
			},
			{
				Args:       []*Type{TAtom},
				QuoteArgs:  map[int]bool{0: true},
				Impl:       Go(undefHandler, RunInCheck()),
				Returns:    []*Type{},
				BarrierPos: -1,
			},
		},
	},
	{
		Name: "var",

		// var SPLICES its body (def/body/undef tokens) onto the tape for the
		// engine to re-step. RunInCheckMode lets the recorder follow that splice
		// so the inline let lowers as the body's events with the bound names as
		// promoted value-def locals (the def/body/undef tokens record exactly as a
		// hand-written `def NAME val end … undef NAME` would). A body word the
		// recorder cannot lower marks the program uncompilable through the same
		// path it does anywhere else, so a refusing body REFUSES rather than
		// producing a silent empty unit.
		Signatures: []Signature{{
			Args:       []*Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       Go(varHandler, RunInCheck()),
			Returns:    []*Type{TAny}, BarrierPos: -1,
		}},
	},
	{
		Name: "fn",

		Signatures: []Signature{{
			Args:       []*Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       Go(fnHandler, RunInCheck()),
			Returns:    []*Type{TFunction},
			BarrierPos: -1,
		}},
	},
	{
		// `afn` is the canonical anonymous-fn constructor. The parser
		// folds `A => B` into the group `(A afn B)`. Sig is [Any Any |]
		// — both args forward-eligible, both typed Any. NoEvalArgs on
		// both so the input sig isn't auto-evaluated and the body's
		// words aren't dispatched before construction. RawParens on the
		// body slot (position 0): a function body is CODE, so a paren
		// body is captured RAW and evaluates per CALL with the params
		// bound — this is what makes the chained arrow curry,
		// `x:Integer => y:Integer => [x add y]` ≡
		// (x ⇒ (y ⇒ body)): the inner lambda constructs inside the
		// outer body, capturing x, instead of eagerly at outer-
		// construction time when x doesn't exist yet. It also lets a
		// paren body reference params at all: `x:Integer => (x mul 2)`.
		Name: "afn",

		Signatures: []Signature{{
			Args:       []*Type{TAny, TAny},
			NoEvalArgs: map[int]bool{0: true, 1: true},
			RawParens:  map[int]bool{0: true},
			Impl:       Go(afnHandler, RunInCheck()),
			Returns:    []*Type{TFunction},
			BarrierPos: -1,
		}},
	},
	{
		Name: "fnsig",

		Signatures: []Signature{{
			Args:       []*Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       Go(fnsigHandler, RunInCheck()),
			// Pure construction — runs in check mode too, so surface
			// schemas carry REAL shapes statically and `exposes` is
			// fully static-checkable (design/SURFACES.10.md S2). A
			// pending gen spec turns the result into a generic
			// fn-shape schema (see the handler).
			Returns: []*Type{TFnUndef}, BarrierPos: -1,
		}},
	},
	{
		Name: "args",

		Signatures: []Signature{{
			Impl:    Go(argsHandler),
			Returns: []*Type{TList}, BarrierPos: -1,
		}},
	},
	{
		Name: "__pa",

		Signatures: []Signature{{
			Impl:    Go(popArgsHandler),
			Returns: []*Type{}, BarrierPos: -1,
		}},
	},
}

// ---- def ----

// installAndRecordDef binds name→value and records the def for check-mode
// unused-def analysis at pos (the def-name token's position). Every
// successful def / typed-def branch ends with this exact pair followed by a
// `return nil, nil`, so it is consolidated here. The optional stackOnly flag
// is forwarded to InstallDef (only the plain `def` path sets it).
func installAndRecordDef(r *Registry, name string, value Value, pos SrcPos, stackOnly ...bool) ([]Value, error) {
	// A def's own INSTALL must not count as a USE of the name. Installing a
	// fn runs its construction-time body pass, which resolves the fn's own
	// name (recursion support) and records a spurious self-use; an uncalled
	// fn would then never be flagged unused now that RecordDef no longer
	// resets uses on rebind. Snapshot this name's use flag across
	// install+record and restore it, so only EXTERNAL references count. A
	// prior legitimate use (e.g. a loop-carried read before a rebind) is
	// preserved (prevUsed stays true); a fresh def's self-use is undone.
	checking := r.Check.IsActive()
	prevUsed := checking && r.Check.DefsUsed != nil && r.Check.DefsUsed[name]
	InstallDef(r, name, value, stackOnly...)
	r.Check.RecordDef(name, pos)
	if checking {
		if r.Check.DefsUsed == nil {
			r.Check.DefsUsed = map[string]bool{}
		}
		if prevUsed {
			r.Check.DefsUsed[name] = true
		} else {
			delete(r.Check.DefsUsed, name)
		}
	}
	// Mark a computed binding for value-def-local promotion in the bytecode
	// lowerer: a named value may be referenced in any order, so its producing
	// event is stored to a frame local rather than left on the simulated stack.
	r.Check.Emit.MarkValueDef(value)
	return nil, nil
}

func defHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := defName(args[0])
	stackOnly := defStackOnly(args[0])
	body := args[1]
	if IsCapitalisedName(name) {
		// `def` is the universal binder (design/TYPE-UNIFORM.10.md
		// Phase 2): a capitalised name is a TYPE binding. Delegate to
		// the kernel type installer — the same path the `type` word
		// uses — so object/predicate lattice-minting and all
		// type-installation validation happen in exactly one place.
		return nil, eng.InstallType(r, name, body)
	}
	if err := ValidateWordName(name); err != nil {
		return nil, fmt.Errorf("def %s: %w", name, err)
	}
	if r.IsBuiltinWord(name) {
		return nil, reservedWordError(r, "def", name)
	}
	if r.Defs.IsType(name) {
		return nil, r.AqlError("def_error", fmt.Sprintf("def %s: name clash — already a type", name), "def")
	}
	return installAndRecordDef(r, name, body, args[0].Pos, stackOnly)
}

// reservedWordError is the error raised when def / undef targets a core
// word (a native / kernel / host-registered word, or a reserved literal
// true/false/none). Core words are frozen — extend the language by
// defining a NEW word, not by shadowing a built-in.
func reservedWordError(r *Registry, op, name string) error {
	return r.AqlError("reserved_word",
		fmt.Sprintf("%s %s: '%s' is a built-in word and cannot be redefined", op, name, name), op)
}

// markRefineDefUncompilable refuses bytecode compilation of a typed-def whose
// constraint is a REFINEMENT — a predicate / DepScalar subset (`def x:(Integer gt
// 10) v`) or a bare-refine newtype (`def x:Pos v`, Pos = refine Integer). The
// interpreter's defTypedHandler validates the predicate and/or reparents the bound
// value to the refine type; the bytecode value-def lowering captures NEITHER (it
// folds `def x:Pos n` to a bare `x≡n` alias keeping the base tag). So a compiled
// `def x:(Integer gt 10) 5` binds x=5 where the interpreter raises, and `def x:Pos
// n` then reports typeof Integer / fails a [Pos] return-check (cluster B of the
// broad miscompile hunt). No store-with-reparent opcode exists, so refuse → fall
// back to the interpreter. Object / alias / schema typed-defs do not route here.
func markRefineDefUncompilable(r *Registry, name string, body Value) {
	// A STATIC (concrete) refinement value's reparent rides the const pool and
	// compiles faithfully — `def p:Pt 5` (a const) folds to a Pt-tagged const, so
	// `p is Pt` holds. Only a DYNAMIC value (a param / computed carrier) loses it:
	// the value-def lowering folds `def x:Pos n` to a bare `x≡n` keeping the base
	// tag, so the compiled `typeof x` / [Pos] return-check see Integer. Refuse only
	// the dynamic case → fall back. (DepScalar's failing-static case is handled at
	// its own branch, since a passing static value needs no reparent.)
	if IsConcrete(body) {
		return
	}
	if es := r.Check.Emit; es != nil && es.Active() {
		es.MarkUncompilable("typed-def `" + name + "`: dynamic refinement reparent/validate is interpreter-only (no compiled store-with-reparent)")
	}
}

// resolveResourceTypeInfo returns the ResourceTypeInfo a typed-def
// annotation denotes, and true, for a Resource/Entity constraint. A
// word-resolved annotation already carries the ResourceTypeInfo body;
// a bare Resource-family type literal (the shape `def e:Entity {…}`
// produces, since builtin type names parse to literals in data context)
// is resolved from the type binding by its lattice-leaf name.
func resolveResourceTypeInfo(r *Registry, constraint Value) (ResourceTypeInfo, bool) {
	if IsResourceType(constraint) {
		info, _ := AsResourceType(constraint)
		return info, true
	}
	// A bare Resource-family type literal (what `def e:Entity {…}`
	// produces — builtin type names parse to literals in data context)
	// carries no ResourceTypeInfo; resolve it from the def binding by
	// the rendered type name. The lookup itself is the guard: only the
	// Resource/Entity def bindings resolve to a ResourceType, so this
	// never hijacks a non-Resource annotation.
	if r != nil && IsBareTypeNode(constraint) {
		if info, ok := lookupResourceTypeByName(r, constraint.String()); ok {
			return info, true
		}
	}
	return ResourceTypeInfo{}, false
}

// lookupResourceTypeByName resolves a Resource/Entity ResourceTypeInfo
// from a type name. Resource and Entity are installed as def bindings
// (installResourceTypes), so — unlike a user class, which is a type
// binding reachable via TopTypeBody — their schema is found through the
// def store.
func lookupResourceTypeByName(r *Registry, name string) (ResourceTypeInfo, bool) {
	if r == nil || name == "" {
		return ResourceTypeInfo{}, false
	}
	v, ok := r.Defs.Top(name)
	if !ok {
		return ResourceTypeInfo{}, false
	}
	info, err := AsResourceType(v)
	if err != nil {
		return ResourceTypeInfo{}, false
	}
	return info, true
}

func defTypedHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	nameMap, _ := AsMap(args[0])
	if nameMap == nil || nameMap.Len() == 0 {
		return nil, r.AqlError("def_error", "def: typed-name map must have exactly one key, got empty/non-concrete map", "def")
	}
	if nameMap.Len() != 1 {
		return nil, fmt.Errorf("def: typed-name map must have exactly one key, got %d", nameMap.Len())
	}
	name := nameMap.Keys()[0]
	if IsCapitalisedName(name) {
		return nil, r.AqlError("def_error", fmt.Sprintf("def %s: def names must not start with a capital letter (capitalised names are reserved for types)", name), "def")
	}
	if err := ValidateWordName(name); err != nil {
		return nil, fmt.Errorf("def %s: %w", name, err)
	}
	if r.IsBuiltinWord(name) {
		return nil, reservedWordError(r, "def", name)
	}
	if r.Defs.IsType(name) {
		return nil, r.AqlError("def_error", fmt.Sprintf("def %s: name clash — already a type", name), "def")
	}
	constraint, _ := nameMap.Get(name)
	// A parenthesised annotation — `def b:(Box of [Integer]) {…}` —
	// evaluates inline (def's NoEvalMapArgs keeps the typed-name map
	// raw, so the ParenExpr arrives unevaluated). Generic
	// instantiations are the main client; any expression producing a
	// single type value works.
	if IsParenExpr(constraint) {
		toks, _ := AsParenExpr(constraint)
		body := make([]Value, len(toks))
		copy(body, toks)
		sub := New(r)
		out, err := sub.Run(body)
		if err != nil {
			return nil, fmt.Errorf("def %s: type annotation: %w", name, err)
		}
		if len(out) != 1 {
			return nil, fmt.Errorf("def %s: type annotation must produce one type, got %d values", name, len(out))
		}
		constraint = out[0]
	}
	// A typed-list/map annotation whose CHILD is a paren expression —
	// `def xs:[:(Pair of [String Integer])] […]` — needs the child
	// evaluated the same way a top-level paren annotation is (the
	// parser leaves it as a raw ParenExpr payload).
	if evaluated, cerr := eng.ResolveChildTypeExpr(r, constraint); cerr != nil {
		return nil, fmt.Errorf("def %s: type annotation: %w", name, cerr)
	} else {
		constraint = evaluated
	}
	var typeName string
	constraint, typeName, _ = r.ResolveTypedNameValue(constraint)
	if !IsTypeBody(constraint) {
		return nil, fmt.Errorf("def %s: type annotation must be a type value, got %s", name, constraint.String())
	}
	describeType := func() string {
		if typeName != "" {
			return typeName
		}
		return constraint.String()
	}
	body := args[1]
	// A generic SCHEMA annotation — `def b:Box {value:42}` — infers
	// its type arguments from the body and instantiates (Phase 7 /
	// D12); the instantiation then flows through the ordinary typed-def
	// branches below (the ObjectType branch constructs class
	// instances, etc.). Uninferable, undefaulted parameters error.
	if IsTypeSchema(constraint) {
		inst, ierr := eng.InferAndInstantiateSchema(r, constraint, body)
		if ierr != nil {
			return nil, fmt.Errorf("def %s: %w", name, ierr)
		}
		constraint = inst
	}
	if constraint.Parent.Equal(TFnUndef) && IsAtom(body) {
		atomName, _ := AsAtom(body)
		if top, ok := r.Defs.Top(atomName); ok {
			if top.Parent.Equal(TFnDef) || top.Parent.Equal(TFunction) {
				body = top
			}
		}
	}
	if constraint.Parent.Equal(TFnDef) || constraint.Parent.Equal(TFunction) {
		out, matched, err := r.RunPredicate(constraint, body)
		if err != nil {
			return nil, fmt.Errorf("def %s: predicate type %s: %w", name, describeType(), err)
		}
		if !matched {
			return nil, fmt.Errorf("def %s: value %s does not satisfy predicate type %s",
				name, body.String(), describeType())
		}
		// Rewrap with the predicate's *Type so dispatch keys off
		// the nominal name. The underlying Data is unchanged —
		// accessors (AsInteger, AsString, …) read the payload the
		// same way — but the Parent change lets the LCA walk find
		// behaviors installed via `behave compare/q (fn
		// [[Positive Positive] …])` etc.
		//
		// Only fires when the predicate declares a concrete input
		// type (e.g. `fn [n:Integer …]`). Predicates with `Any`
		// input — the historical `fn [x:Any Any […]]` shape — are
		// pure validation gates: their *Type is parented at
		// TFnDef and rewrapping would break rendering and
		// downstream type tests (the value would print as
		// `Type/Function/Bbd({…})` rather than its underlying
		// scalar). The PredicateInputType check below mirrors the
		// InstallType decision so the two paths stay aligned.
		if typeName != "" && eng.PredicateInputType(constraint) != nil {
			if def := r.LookupTypeName(typeName); def != nil && def.Origin != eng.OriginBuiltin {
				out = ReparentValue(out, def)
			}
		}
		markRefineDefUncompilable(r, name, body)
		return installAndRecordDef(r, name, out, args[0].Pos)
	}

	// ObjectType constraint (`def x:Person {map}` where Person is
	// `type Person object {…}`): build a Person-typed ObjectInstance
	// from the body map via make-style construction. This closes the
	// "structural for validation, nominal for dispatch" gap for
	// object types — without this branch the value would have
	// Parent=TMap and Person's registered behaviors would never
	// dispatch. The result carries Parent=Person, satisfies the
	// `behave compare/q (fn [[Person Person] …])` dispatch path, and
	// supports `get`/`set` via the ObjectInstance signatures.
	//
	// Accepts both a raw Map (built via make) and an already-typed
	// ObjectInstance (passed through). Other body shapes fall
	// through to Unify and either succeed or surface a type error.
	if IsClassType(constraint) {
		info, _ := AsClassType(constraint)
		if body.Parent.Equal(TMap) {
			// `def b:Type {map}` is `def b (make Type map)`. In emit mode record
			// the make event the direct MakeObject call would skip, so the bound
			// instance has the same provenance an explicit make gives it (a
			// downstream `b typeof` then compiles). Outside emit mode this is a
			// no-op and the concrete instance is bound.
			if carrier, ok := eng.RecordTypedDefMake(r, constraint, body, args[0].Pos); ok {
				return installAndRecordDef(r, name, carrier, args[0].Pos)
			}
			result, err := eng.MakeObject(info, body, r)
			if err != nil {
				return nil, fmt.Errorf("def %s: %w", name, err)
			}
			return installAndRecordDef(r, name, result[0], args[0].Pos)
		}
		if IsClassInstance(body) {
			oi, _ := AsClassInstance(body)
			// Accept if the instance's nominal type matches the
			// declared one (covers `def x:Person make Person {…}`).
			if oi.TypeRef != nil && oi.TypeRef.ID == info.ID {
				return installAndRecordDef(r, name, body, args[0].Pos)
			}
		}
	}
	// Resource/Entity annotation: `def e:Entity {map}` is `def e (make
	// Entity map)` — the same construction path a class annotation takes,
	// routed through MakeResource (Resource/Entity have their own flat
	// instance struct). make's `[Ideal Map]` sig serves both kinds, so
	// the emit-mode RecordTypedDefMake carrier is recorded identically.
	// Unlike user class names (which parse as Words and resolve to their
	// ClassTypeInfo), the builtin Resource/Entity names parse to BARE
	// type literals in data context, so resolveResourceTypeInfo also
	// looks the schema up by name when the constraint carries no body.
	if resInfo, isRes := resolveResourceTypeInfo(r, constraint); isRes {
		if body.Parent.Equal(TMap) {
			if carrier, ok := eng.RecordTypedDefMake(r, constraint, body, args[0].Pos); ok {
				return installAndRecordDef(r, name, carrier, args[0].Pos)
			}
			provided, merr := AsMutableMap(body)
			if merr != nil {
				return nil, fmt.Errorf("def %s: %w", name, merr)
			}
			result, err := MakeResource(resInfo, provided, r)
			if err != nil {
				return nil, fmt.Errorf("def %s: %w", name, err)
			}
			return installAndRecordDef(r, name, result[0], args[0].Pos)
		}
		if IsResourceInstance(body) {
			ri, _ := AsResourceInstance(body)
			// Accept a pre-made instance whose nominal type matches.
			if ri.TypeRef != nil && ri.TypeRef.ID == resInfo.ID {
				return installAndRecordDef(r, name, body, args[0].Pos)
			}
		}
	}
	if r.Check.IsActive() && constraint.IsDepScalar() && !IsConcrete(body) {
		if body.Parent.ConformsTo(constraint.Parent) {
			// An ABSTRACT (carrier) body admits on base conformance only —
			// the predicate is value-level and the value is unknown here, so
			// validation stays at RUNTIME via v.Is. A compiled `def
			// x:(Integer gt 10) (f 1)` would bind whatever f returns where
			// the interpreter may raise, and the compiler can't run the
			// inline predicate (no canonical node carrying the DepScalar
			// Behavior) — refuse abstract DepScalar typed-defs → fall back.
			//
			// A CONCRETE body deliberately falls THROUGH to the Unify below:
			// unifyDepScalar runs the self-contained predicate on the real
			// value, so `def x:(Integer gt 10) 5` is flagged at check time
			// with the byte-identical runtime message (and a passing literal
			// binds the same value in both engines, so it stays compilable).
			if es := r.Check.Emit; es != nil && es.Active() {
				es.MarkUncompilable("typed-def `" + name + "`: DepScalar predicate validation is interpreter-only")
			}
			return installAndRecordDef(r, name, body, args[0].Pos)
		}
	}
	// User-minted bare-refine subtype (`def Foo refine Integer`): the
	// constraint is the Foo type literal whose lattice Parent is the
	// type Foo refines. Check the body satisfies the parent type
	// (since values of the parent type are the inhabitants Foo can
	// accept), then reparent a COPY of the body to Foo. Mutating the
	// Unify result would store its by-value type literal (Unify swaps
	// when one side is bare and subtype-ordered) instead of the
	// body's payload — `def x:Foo 1` would silently bind x to the
	// Foo-tagged type literal, not the integer 1.
	if IsBareTypeNode(constraint) && constraint.Origin == eng.OriginUserDef &&
		typeName != "" && constraint.Parent != nil {
		if def := r.LookupTypeName(typeName); def != nil && def.Origin == eng.OriginUserDef {
			// Walk up the lattice past any intervening user refines
			// (e.g. `Foo refine Item refine String`) to the nearest
			// builtin ancestor and unify against THAT. A sibling-of-
			// constraint kernel subtype (ProperString satisfying a
			// Foo whose parent Item branches off String) wouldn't
			// match the immediate parent literal but does match the
			// shared kernel base.
			root := def.Parent
			for root != nil && root.Origin == eng.OriginUserDef {
				root = root.Parent
			}
			if root == nil {
				return nil, fmt.Errorf("def %s: refine subtype %s has no builtin ancestor",
					name, describeType())
			}
			parentLit := NewTypeLiteral(root)
			if _, ok := Unify(body, parentLit); ok {
				markRefineDefUncompilable(r, name, body)
				return installAndRecordDef(r, name, ReparentValue(body, def), args[0].Pos)
			}
			if r.Check.IsActive() {
				r.Check.AddDiagnostic(CheckDiagnostic{
					Code: "type_error",
					Detail: fmt.Sprintf("def %s: value %s does not unify with declared type %s",
						name, body.String(), describeType()),
					Word: name,
					Row:  args[0].Pos.Row,
					Col:  args[0].Pos.Col,
				})
				return installAndRecordDef(r, name, NewCarrier(def), args[0].Pos)
			}
			return nil, fmt.Errorf("def %s: value %s does not unify with declared type %s",
				name, body.String(), describeType())
		}
	}
	unified, ok := Unify(body, constraint)
	if !ok {
		if r.Check.IsActive() {
			r.Check.AddDiagnostic(CheckDiagnostic{
				Code: "type_error",
				Detail: fmt.Sprintf("def %s: value %s does not unify with declared type %s",
					name, body.String(), describeType()),
				Word: name,
				Row:  args[0].Pos.Row,
				Col:  args[0].Pos.Col,
			})
			return installAndRecordDef(r, name, NewCarrier(constraint.Parent), args[0].Pos)
		}
		return nil, fmt.Errorf("def %s: value %s does not unify with declared type %s",
			name, body.String(), describeType())
	}
	// FnUndef constraint (`def f:Mapper fn […]`): after Unify
	// confirms the function shape matches Mapper, rewrap the
	// Parent so dispatch keys off Mapper rather than the generic
	// TFunction / TFnDef. Behaviors installed via
	// `behave compare/q (fn [[Mapper Mapper] …])` then dispatch on
	// f. Same rewrap pattern as predicate types — the payload
	// shape (FnDefInfo) is unchanged, accessors keep working, just
	// the dispatch identity flips.
	if constraint.Parent.Equal(TFnUndef) && typeName != "" {
		if def := r.LookupTypeName(typeName); def != nil && def.Origin != eng.OriginBuiltin {
			unified = ReparentValue(unified, def)
		}
	}
	return installAndRecordDef(r, name, unified, args[0].Pos)
}

// ---- undef ----

func undefHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := defName(args[0])
	if r.IsBuiltinWord(name) {
		return nil, reservedWordError(r, "undef", name)
	}
	if IsCapitalisedName(name) {
		// `undef` is the universal unbinder (the symmetric completion
		// of Phase 2's universal `def` — design/TYPE-UNIFORM.10.md):
		// a capitalised name is a TYPE binding, so pop it from the single
		// binding store and retire the minted lattice type.
		entry, ok := r.Defs.PopEntry(name)
		if !ok {
			return nil, r.AqlError("undef_error",
				fmt.Sprintf("undef %s: no such type binding", name), "undef")
		}
		if entry.TypeDef != nil {
			r.Types.Retire(entry.TypeDef)
		}
		return nil, nil
	}
	UninstallDef(r, name)
	return nil, nil
}

func undefFnHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := defName(args[0])
	undefInfo, ok := args[1].Data.(FnUndefInfo)
	if !ok {
		return nil, fmt.Errorf("undef: expected fn undef spec, got %s", args[1].String())
	}
	UninstallFnSigs(r, name, undefInfo)
	return nil, nil
}

// ---- var ----

func varHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	list := args[0]
	if !list.Parent.Equal(TList) {
		return nil, r.AqlError("var_error", "var: argument must be a list", "var")
	}
	if !IsConcrete(list) {
		return nil, r.AqlError("var_error", "var: argument must be a concrete list, got type literal", "var")
	}
	elems, _ := AsList(list)
	if elems.Len() == 0 {
		return nil, r.AqlError("var_error", "var: empty list", "var")
	}

	declVal := elems.Get(0)
	if !declVal.Parent.Equal(TList) || !IsConcrete(declVal) {
		return nil, r.AqlError("var_error", "var: first element must be a list of variable declarations", "var")
	}
	decls, _ := AsList(declVal)
	body := elems.Slice()[1:]

	var result []Value
	var varNames []string

	for _, decl := range decls.Slice() {
		switch {
		case IsWord(decl):
			_as0, _ := AsWord(decl)
			name := _as0.Name
			varNames = append(varNames, name)
			result = append(result, NewWord("def"), NewWord(name), NewEnd())

		case decl.Parent.Equal(TList) && decl.Data != nil:
			declElems, _ := AsList(decl)
			if declElems.Len() < 2 {
				return nil, r.AqlError("var_error", "var: declaration list must have name and value", "var")
			}
			var name string
			if IsWord(declElems.Get(0)) {
				_as1, _ := AsWord(declElems.Get(0))
				name = _as1.Name
			} else if declElems.Get(0).Parent.ConformsTo(TString) {
				name, _ = AsString(declElems.Get(0))
			} else {
				return nil, r.AqlError("var_error", "var: declaration name must be a word or string", "var")
			}
			varNames = append(varNames, name)
			result = append(result, NewWord("def"), NewWord(name))
			result = append(result, declElems.Slice()[1:]...)
			result = append(result, NewEnd())

		case decl.Parent.ConformsTo(TString):
			name, _ := AsString(decl)
			varNames = append(varNames, name)
			result = append(result, NewWord("def"), NewWord(name), NewEnd())

		default:
			return nil, fmt.Errorf("var: invalid declaration: %s", decl.String())
		}
	}

	result = append(result, body...)

	// Cleanup via __varundef (not `undef`): the body residual is still on the
	// stack here, and `undef`'s 2-arg fn-overload form would mis-match it in
	// check mode (a dynamic-Any residual gradually satisfies TFnUndef). The
	// dedicated 1-arg word dispatches identically in check and at runtime, which
	// is what lets a var-body compile to a closure unit.
	for i := len(varNames) - 1; i >= 0; i-- {
		result = append(result, NewWord("__varundef"), NewWord(varNames[i]))
	}

	return result, nil
}

// ---- fn ----

// fnHandler always produces a Function value. The list must be a
// non-zero multiple of 3 (input/output/body triples). For the
// type-only / shape form (input/output pairs, no body) use the
// separate `fnsig` word — registered via eng.RegisterCoreFnSig
// from register.go.
func fnHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	// A pending gen spec (`def identity gen [T] fn [[x:T] [T] [x]]`)
	// makes this a GENERIC fn: the placeholders stay bound while
	// ParseFnParams resolves the sigs (so `x:T` types against the
	// placeholder node, whose Behavior handles dispatch admission),
	// then pop; the spec rides FnDefInfo.Gen so each call installs
	// the inferred body-scoped type bindings.
	genSpec := r.TakePendingGen()
	failGen := func(err error) ([]Value, error) {
		if genSpec != nil {
			PopGenBindings(r, genSpec)
		}
		return nil, err
	}
	list := args[0]
	if !list.Parent.Equal(TList) {
		return failGen(r.AqlError("fn_error", "fn: argument must be a list", "fn"))
	}
	if !IsConcrete(list) {
		return failGen(r.AqlError("fn_error", "fn: argument must be a concrete list, got type literal", "fn"))
	}
	_lst, _ := AsList(list)
	elems := _lst.Slice()
	if len(elems) == 0 || len(elems)%3 != 0 {
		return failGen(r.AqlError("fn_error", "fn: list length must be a non-zero multiple of 3 (input output body triples); use `fnsig` for the type-only form", "fn"))
	}
	fnDef, err := parseFnDef(r, elems)
	if err != nil {
		return failGen(err)
	}
	if genSpec != nil {
		PopGenBindings(r, genSpec)
		fnDef.Gen = genSpec
	}
	// Compute lexical captures: per-sig walks merged into one list.
	// Nil at top-level (no enclosing fn) — natural no-op via
	// ComputeCaptures' baseline check.
	perSig := make([][]CapturedBinding, len(fnDef.Signatures))
	for i := range fnDef.Signatures {
		perSig[i] = eng.ComputeCaptures(r, &fnDef.Signatures[i])
	}
	fnDef.Captured = eng.MergeCaptures(perSig)

	// Check mode, generic fns: declaration-time ABSTRACT check
	// (Phase 5). Analyse each body once with carrier args of the
	// declared param types — for a `x:T` param that is a carrier of
	// the PLACEHOLDER node, so operations on it are admitted exactly
	// when the parameter's bound justifies them (§9.4: a
	// `(T extends Number)` carrier reaches Number ops through the
	// placeholder's lattice parent; a bare T admits nothing it can't
	// prove). Body diagnostics (undefined words, unjustified ops)
	// surface at the definition instead of waiting for a first call.
	// The placeholder bindings are re-pushed around the analysis so
	// body-internal `of [T]` resolves (to a deferred GenInstRef).
	// Non-generic fns get an equivalent construction-time analysis
	// via the dynamic-help example generator; generic params have no
	// synthesizable example values, hence this explicit path.
	if r.Check.IsActive() && genSpec != nil {
		// Names of UNCONSTRAINED type parameters (`gen [T]`, no `extends`): a
		// value of such a type is statically unknown — like an explicit `Any` —
		// so a body word over the param must match gradually, not fail
		// no_signature against a strict abstract carrier.
		unconstrained := map[string]bool{}
		for _, gp := range genSpec.Params {
			if !gp.HasBound {
				unconstrained[gp.Name] = true
			}
		}
		PushGenBindings(r, genSpec)
		for i := range fnDef.Signatures {
			s := &fnDef.Signatures[i]
			if len(s.Body()) == 0 {
				continue
			}
			paramNames := make([]string, len(s.Params))
			carrierArgs := make([]Value, len(s.Params))
			for j, p := range s.Params {
				paramNames[j] = p.Name
				t := p.Type
				if t == nil {
					t = TAny
				}
				// A plain-Any param, or one typed by an unconstrained type
				// parameter, binds a DYNAMIC (gradual) carrier so a body word
				// over it (`b dot value`) matches optimistically instead of
				// failing no_signature — mirroring the gradual treatment
				// ParamInputCarrier gives an `Any` param, extended to the
				// type-parameter case this construction-time generic check hits.
				// A CONCRETE or BOUNDED (`T extends C`) param keeps a strict
				// carrier so its real shape is still checked against the bound;
				// a genuine misuse or undefined word in the body still surfaces.
				if t.Equal(TAny) || eng.IsUnconstrainedTypeParam(t) || unconstrained[t.Leaf()] {
					// A value of an unconstrained type parameter (or Any) is
					// statically ANY type, so bind dynamic(Any) — a body word
					// then matches gradually. dynamic(T) is NOT enough: the
					// gradual receiver match keys on an Any bound, so a `dot` over
					// a dynamic(T) carrier still misses every concrete slot.
					carrierArgs[j] = NewDynamicCarrier(TAny)
				} else {
					carrierArgs[j] = NewCarrier(t)
				}
			}
			eng.AnalyseFnBody(r, "", paramNames, s.Body(), carrierArgs, fnDef.Captured, s.Returns)
		}
		PopGenBindings(r, genSpec)
	}

	// Check mode: flag overloads that an earlier, higher-priority
	// signature already subsumes — under first-match-wins dispatch they
	// can never fire (the dead-clause analogue). A static property of the
	// sig list, emitted once at fn construction.
	if r.Check.IsActive() && len(fnDef.Signatures) > 1 {
		for _, d := range eng.DeadSignatures(fnDef.Signatures) {
			r.Check.AddDiagnostic(eng.CheckDiagnostic{
				Code:   "unreachable_signature",
				Detail: "fn overload " + fnSigArgList(d.Sig) + " is unreachable — the earlier signature " + fnSigArgList(d.ShadowedBy) + " already accepts every call it would match",
				Word:   "fn",
			})
		}
	}

	return []Value{NewFunction(fnDef)}, nil
}

// fnSigArgList renders a signature's argument types as a short
// `[Integer String]` list for the unreachable_signature diagnostic.
func fnSigArgList(s eng.Signature) string {
	ts := s.ArgTypes()
	parts := make([]string, len(ts))
	for i, t := range ts {
		if t == nil {
			parts[i] = "Any"
			continue
		}
		parts[i] = t.Leaf()
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// afnHandler — `afn input body` constructs an anonymous Function value
// with a single signature. Mirrors the per-triple shape of ParseFnDef
// (eng/go/fn_def.go) for one triple: auto-wraps non-list input and body
// into single-element lists, parses params via the shared
// eng.ParseFnParams, and constructs the FnSig with Returns=[TAny] and
// Anonymous=true. Static Returns is conservative so call sites that
// inspect the type without invoking see `Any`; check-mode dispatch
// reads the Anonymous flag and runs AnalyseFnBody for real propagation.
func afnHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	// `afn` is normally encountered as the swap form `input afn body`
	// (because `input => body` desugars to this), which makes args[1]
	// the source-left operand (input sig) and args[0] the source-right
	// operand (body). Mirrors the AQL `args[1] op args[0]` convention.
	inputSig := args[1]
	body := args[0]

	if !inputSig.Parent.Equal(TList) {
		inputSig = NewList([]Value{inputSig})
	}

	params, barrierPos, err := parseFnParams(r, inputSig)
	if err != nil {
		return nil, r.AqlError("afn_error", err.Error(), "afn")
	}

	var bodyElems []Value
	if body.Parent.Equal(TList) && body.Data != nil {
		lst, _ := AsList(body)
		bodyElems = lst.Slice()
	} else {
		bodyElems = []Value{body}
	}

	sig := FnSig{
		Params:     params,
		Returns:    []*Type{TAny},
		Impl:       AQL(bodyElems),
		BarrierPos: barrierPos,
		QuoteArgs:  eng.QuoteArgsFromParams(params),
	}
	fnDef := FnDefInfo{
		Signatures: []FnSig{sig},
		Anonymous:  true,
		Captured:   eng.ComputeCaptures(r, &sig),
	}
	return []Value{NewFunction(fnDef)}, nil
}

// fnsigHandler — `fnsig [input output …]` produces a function-SHAPE
// type literal (FnUndef) from input/output sig pairs. The type-only
// counterpart to `fn` — same grammar, no body. The list length must
// be a non-zero multiple of 2 (each pair is one signature). The
// result is an FnUndef value usable as a type constraint, e.g.
// `def f:fnsig [[Integer] [String]] impl` asserts that `impl` is a
// function whose signatures cover the shape `Integer → String`.
//
// FnUndef is structural: any function value whose registered
// signatures satisfy every pair in the FnUndef matches. See
// eng/go/fnsig.go::FnUndefMatchesFnDef.
func fnsigHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, &AqlError{
			Code:   "fnsig_invalid_spec",
			Detail: "fnsig: argument must be a concrete list",
		}
	}
	_lst, _ := AsList(args[0])
	spec := _lst.Slice()
	if len(spec) == 0 || len(spec)%2 != 0 {
		return nil, &AqlError{
			Code:   "fnsig_invalid_spec",
			Detail: "fnsig: list length must be a non-zero multiple of 2 (input output pairs); use `fn` for the with-body form",
		}
	}
	info, err := parseFnUndefSpec(r, spec)
	if err != nil {
		if g := r.TakePendingGen(); g != nil {
			PopGenBindings(r, g)
		}
		return nil, err
	}
	// A pending gen spec turns the shape into a generic fn-shape
	// schema (`def Mapper gen [T U] fnsig [[T] [U]]`): the
	// placeholders were live while ParseFnParams resolved T/U above.
	if g := r.TakePendingGen(); g != nil {
		return genWrapSchema(r, g, NewFnUndef(info), SchemaFnSig)
	}
	return []Value{NewFnUndef(info)}, nil
}

// ---- args / __pa ----

func argsHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	top, ok, err := r.Args.Top()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, r.AqlError("args_error", "args: not inside a function", "args")
	}
	return []Value{top}, nil
}

func popArgsHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	// The Args pop and the FnBaseline pop must move together (closure-
	// capture detection on subsequent fn constructions reads the
	// baseline). eng.PopFrameArgs is the single home of that pairing,
	// shared with any eager frame teardown, so the two cannot drift.
	if err := eng.PopFrameArgs(r); err != nil {
		return nil, err
	}
	return nil, nil
}
