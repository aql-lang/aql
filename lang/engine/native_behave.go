package engine

import (
	"errors"
	"fmt"

	"github.com/aql-lang/aql/eng"
)

// behave BEHAVIOR FN
//
// Installs a user-defined capability on a type. The first arg is the
// behavior name (Atom — `compare/q`, `canon/q`, `nodify/q`, …); the
// second is a Function whose sig declares the target type and shape:
//
//	behave compare/q (fn [[Foo Foo] [Integer] [body]])
//	behave canon/q   (fn [[Foo]     [String]  [body]])
//	behave nodify/q  (fn [[Foo]     [Any]     [body]])
//
// The handler validates the fn's first sig against the behavior's
// declared shape, extracts the target type from the input params,
// looks the type up in the registry, and wraps its TypeBehavior so
// the body runs whenever the kernel dispatches the corresponding
// capability (CompareValues for compare, Value.String for canon,
// NodifyValue for nodify).
//
// Calling `behave` again on the same type with the same or a
// different behavior is additive — the existing userBehavior wrapper
// accepts new capability slots without losing previously installed
// ones.
var behaveNative = NativeFunc{
	Name:        "behave",
	ForwardArgs: true,
	Signatures: []NativeSig{
		{
			Args:      []*Type{TAtom, TFunction},
			QuoteArgs: map[int]bool{0: true},
			Handler:   behaveHandler,
			Returns:   []*Type{},
		},
		// String form for the behavior name (`behave "compare" fn […]`).
		{
			Args:    []*Type{TString, TFunction},
			Handler: behaveHandler,
			Returns: []*Type{},
		},
	},
}

// tonode VALUE
//
// Project VALUE into a Node or Scalar via its type's Nodifier
// capability — direct access to the data-shape produced by a
// `behave nodify/q` body without the JSON-string serialisation step.
// Useful when the caller wants the structural result for further AQL
// processing rather than for wire output. With no nodify behavior
// registered for the type, the value passes through unchanged.
//
// The existing `jsonify` word composes this projection with
// voxgig/struct's JSON encoder; `tonode` exposes just the projection
// so tests and downstream pipelines can observe the Node/Scalar
// directly.
var tonodeNative = NativeFunc{
	Name:        "tonode",
	ForwardArgs: true,
	Signatures: []NativeSig{{
		Args: []*Type{TAny},
		Handler: func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			out, err := eng.NodifyValue(args[0])
			if err != nil {
				return nil, err
			}
			return []Value{out}, nil
		},
		Returns: []*Type{TAny},
	}},
}

// behaviorEntry describes how `behave` should validate the supplied
// fn and where the resulting body lives on the target Type's wrapper.
type behaviorEntry struct {
	// validate inspects the fn's first sig and returns the target
	// *Type the body should be attached to, or an error if the shape
	// doesn't match the behavior's contract.
	validate func(sig eng.FnSig) (*eng.Type, error)
	// install mutates the userBehavior wrapper to record the body
	// under the appropriate capability slot.
	install func(u *userBehavior, body []eng.Value)
}

// behaviors is the kernel-known table of behavior names → wiring
// rules. The registry is closed: `behave unknown/q fn […]` errors
// out rather than silently installing a behavior the kernel doesn't
// dispatch. Plugins can extend the table at init time (not in this
// commit).
var behaviors = map[string]behaviorEntry{
	"compare": {
		validate: validateCompareSig,
		install:  func(u *userBehavior, body []eng.Value) { u.compareBody = body },
	},
	"canon": {
		validate: validateCanonSig,
		install:  func(u *userBehavior, body []eng.Value) { u.canonBody = body },
	},
	"nodify": {
		validate: validateNodifySig,
		install:  func(u *userBehavior, body []eng.Value) { u.nodifyBody = body },
	},
}

func behaveHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := defName(args[0])
	be, ok := behaviors[name]
	if !ok {
		return nil, fmt.Errorf("behave %s: unknown behavior name; known: compare, canon, nodify", name)
	}

	fnVal := args[1]
	info, err := extractFnDefInfo(fnVal)
	if err != nil {
		return nil, fmt.Errorf("behave %s: %w", name, err)
	}
	if len(info.Sigs) == 0 {
		return nil, fmt.Errorf("behave %s: fn has no signatures", name)
	}
	sig := info.Sigs[0]

	target, err := be.validate(sig)
	if err != nil {
		return nil, fmt.Errorf("behave %s: %w", name, err)
	}
	if target == nil {
		return nil, fmt.Errorf("behave %s: could not infer target type from fn sig", name)
	}
	if target.Origin == eng.OriginBuiltin {
		return nil, fmt.Errorf("behave %s: cannot install on builtin type %s", name, target.Leaf())
	}

	body := append([]Value{}, sig.Body...)

	// Reuse an existing userBehavior wrapper if one is already
	// installed — adding a new capability (canon on top of compare)
	// must preserve previously installed slots.
	var ub *userBehavior
	if existing, ok := target.Behavior.(*userBehavior); ok {
		ub = existing
	} else {
		ub = &userBehavior{
			prev:     target.Behavior,
			registry: r,
			typeName: target.Leaf(),
		}
		target.Behavior = ub
	}
	be.install(ub, body)
	return nil, nil
}

// extractFnDefInfo unwraps a TFunction or TFnDef value into its
// FnDefInfo payload. Returns an error for anything else.
func extractFnDefInfo(v Value) (eng.FnDefInfo, error) {
	if v.VType == nil {
		return eng.FnDefInfo{}, errors.New("fn arg is nil")
	}
	if !v.VType.Equal(eng.TFunction) && !v.VType.Equal(eng.TFnDef) {
		return eng.FnDefInfo{}, fmt.Errorf("fn arg must be a Function (got %s)", v.VType.String())
	}
	info, ok := v.Data.(eng.FnDefInfo)
	if !ok {
		return eng.FnDefInfo{}, fmt.Errorf("fn arg has invalid payload (%T)", v.Data)
	}
	return info, nil
}

// validateCompareSig enforces shape `[[T T] [Integer] [body]]` and
// returns T. Both input params must be the same type; the kernel will
// only dispatch the comparator when both operands share an ancestor
// at or below T.
func validateCompareSig(sig eng.FnSig) (*eng.Type, error) {
	if len(sig.Params) != 2 {
		return nil, fmt.Errorf("compare: fn must take 2 args (got %d)", len(sig.Params))
	}
	if len(sig.Returns) != 1 || !sig.Returns[0].Equal(eng.TInteger) {
		return nil, fmt.Errorf("compare: fn must return Integer")
	}
	t0 := sig.Params[0].Type
	t1 := sig.Params[1].Type
	if t0 == nil || t1 == nil {
		return nil, fmt.Errorf("compare: both params must declare a type")
	}
	if !t0.Equal(t1) {
		return nil, fmt.Errorf("compare: both params must be the same type (got %s and %s)", t0, t1)
	}
	return t0, nil
}

// validateCanonSig enforces shape `[[T] [String] [body]]` and returns
// T. The body produces a canonical-source string for values of type T.
func validateCanonSig(sig eng.FnSig) (*eng.Type, error) {
	if len(sig.Params) != 1 {
		return nil, fmt.Errorf("canon: fn must take 1 arg (got %d)", len(sig.Params))
	}
	if len(sig.Returns) != 1 || !sig.Returns[0].Equal(eng.TString) {
		return nil, fmt.Errorf("canon: fn must return String")
	}
	t := sig.Params[0].Type
	if t == nil {
		return nil, fmt.Errorf("canon: param must declare a type")
	}
	return t, nil
}

// validateNodifySig enforces shape `[[T] [Any] [body]]` and returns
// T. The body produces a Node or Scalar projection of the value —
// the output stays in the AQL data domain (Integer, String, Map,
// List, …) rather than a serialised JSON string, so callers can
// compose with other data transforms before encoding.
func validateNodifySig(sig eng.FnSig) (*eng.Type, error) {
	if len(sig.Params) != 1 {
		return nil, fmt.Errorf("nodify: fn must take 1 arg (got %d)", len(sig.Params))
	}
	if len(sig.Returns) != 1 {
		return nil, fmt.Errorf("nodify: fn must declare a single return type")
	}
	t := sig.Params[0].Type
	if t == nil {
		return nil, fmt.Errorf("nodify: param must declare a type")
	}
	return t, nil
}

// userBehavior is the shared wrapper type that carries one or more
// AQL-bodied capability slots on a target *Type. The TypeBehavior
// surface (Match / Format / Equal) delegates to the previous
// Behavior; capability methods (Compare for compare, Format-via-canon
// for canon, Nodify for nodify) run an installed body or hand off
// to prev.
//
// Re-entrancy: a canon body that triggers another render of the same
// VType (e.g. by calling `inspect` or formatting a nested field of
// the same type) loops through Value.String → Behavior.Format → body
// → Value.String. The per-VType inRender guard breaks the loop by
// falling back to the previous Behavior's Format on re-entry. A
// parallel inNodify guard handles `tonode`-bodies that recurse into
// the same VType. The guards are scoped to the wrapper —
// concurrent rendering across goroutines doesn't share them, matching
// the kernel's general single-goroutine engine model.
type userBehavior struct {
	prev        eng.TypeBehavior
	registry    *Registry
	typeName    string
	compareBody []Value
	canonBody   []Value
	nodifyBody  []Value
	inRender    bool
	inNodify    bool
}

// Match delegates to prev — `behave` does not currently install
// custom Match logic; that's the job of predicate types and external
// builtins.
func (u *userBehavior) Match(v Value, t *Type) bool {
	if u.prev != nil {
		return u.prev.Match(v, t)
	}
	return eng.DefaultBehavior.Match(v, t)
}

// Equal also delegates — equality stays structural by default.
func (u *userBehavior) Equal(a, b Value) bool {
	if u.prev != nil {
		return u.prev.Equal(a, b)
	}
	return eng.DefaultBehavior.Equal(a, b)
}

// Format runs the installed canon body if any, otherwise delegates
// to prev. On re-entry (a canon body triggering another render of
// the same VType), falls back to prev to break the loop.
func (u *userBehavior) Format(v Value) string {
	if len(u.canonBody) == 0 || u.inRender {
		if u.prev != nil {
			return u.prev.Format(v)
		}
		return eng.DefaultBehavior.Format(v)
	}
	u.inRender = true
	defer func() { u.inRender = false }()
	s, err := u.runCanonBody(v)
	if err != nil {
		return fmt.Sprintf("<%s canon-error: %v>", u.typeName, err)
	}
	return s
}

// Compare runs the installed comparator body if any. If no body is
// installed but prev has its own Comparer, delegate to it. Otherwise
// signal `ErrNoComparer` so CompareValues continues the parent walk.
func (u *userBehavior) Compare(a, b Value) (int, error) {
	if len(u.compareBody) > 0 {
		return u.runCompareBody(a, b)
	}
	if cmp, ok := u.prev.(eng.Comparer); ok {
		return cmp.Compare(a, b)
	}
	return 0, eng.ErrNoComparer
}

func (u *userBehavior) runCompareBody(a, b Value) (int, error) {
	r := u.registry
	if r == nil {
		return 0, fmt.Errorf("behave compare %s: no registry attached", u.typeName)
	}
	r.Defs.Push("a", a)
	r.Defs.Push("b", b)
	defer r.Defs.Pop("a")
	defer r.Defs.Pop("b")

	tokens := append([]Value{}, u.compareBody...)
	sub := eng.NewTop(r)
	result, err := sub.Run(tokens)
	if err != nil {
		return 0, fmt.Errorf("behave compare %s: %w", u.typeName, err)
	}
	if len(result) == 0 {
		return 0, fmt.Errorf("behave compare %s: body produced no result", u.typeName)
	}
	top := result[len(result)-1]
	if !top.VType.Matches(eng.TInteger) {
		return 0, fmt.Errorf("behave compare %s: body must return Integer, got %s", u.typeName, top.VType.String())
	}
	n, err := eng.AsInteger(top)
	if err != nil {
		return 0, fmt.Errorf("behave compare %s: %w", u.typeName, err)
	}
	switch {
	case n < 0:
		return -1, nil
	case n > 0:
		return 1, nil
	default:
		return 0, nil
	}
}

// Nodify runs the installed projection body if any. Falls back to
// prev's Nodifier when present, or signals ErrNoNodifier so
// NodifyValue continues the parent-chain walk. Re-entrancy is
// guarded the same way as Format — a nodify body that calls
// `tonode` on a nested value of the same type would otherwise loop.
func (u *userBehavior) Nodify(v Value) (Value, error) {
	if len(u.nodifyBody) == 0 {
		if n, ok := u.prev.(eng.Nodifier); ok {
			return n.Nodify(v)
		}
		return Value{}, eng.ErrNoNodifier
	}
	if u.inNodify {
		// Re-entry on the same type — fall through so the body
		// can use `tonode` on nested fields of the same type
		// without looping. Returning ErrNoNodifier here would
		// just propagate up; falling back to the value itself is
		// the natural "no further transformation" signal.
		return v, nil
	}
	u.inNodify = true
	defer func() { u.inNodify = false }()
	return u.runNodifyBody(v)
}

func (u *userBehavior) runNodifyBody(v Value) (Value, error) {
	r := u.registry
	if r == nil {
		return Value{}, fmt.Errorf("behave nodify %s: no registry attached", u.typeName)
	}
	r.Defs.Push("a", v)
	defer r.Defs.Pop("a")

	tokens := append([]Value{}, u.nodifyBody...)
	sub := eng.NewTop(r)
	result, err := sub.Run(tokens)
	if err != nil {
		return Value{}, fmt.Errorf("behave nodify %s: %w", u.typeName, err)
	}
	if len(result) == 0 {
		return Value{}, fmt.Errorf("behave nodify %s: body produced no result", u.typeName)
	}
	return result[len(result)-1], nil
}

func (u *userBehavior) runCanonBody(v Value) (string, error) {
	r := u.registry
	if r == nil {
		return "", fmt.Errorf("no registry attached")
	}
	r.Defs.Push("a", v)
	defer r.Defs.Pop("a")

	tokens := append([]Value{}, u.canonBody...)
	sub := eng.NewTop(r)
	result, err := sub.Run(tokens)
	if err != nil {
		return "", err
	}
	if len(result) == 0 {
		return "", fmt.Errorf("body produced no result")
	}
	top := result[len(result)-1]
	if !top.VType.Matches(eng.TString) {
		return "", fmt.Errorf("body must return String, got %s", top.VType.String())
	}
	s, err := eng.AsString(top)
	if err != nil {
		return "", err
	}
	return s, nil
}
