package eng

// Code-body closure compilation (plan P2): a higher-order word whose body is a
// quoted code list compiles that body to its own fn unit, and the dispatch
// records the body operand as a closure (OpPushClosure) instead of a Stage-5
// interpreter island. At run time the word's native handler invokes the
// closure through the VM's re-entrant runner (vmContext.invokeClosure) via the
// InvokeBody seam — no interpreter sub-engine.

// callableWord describes such a word: bodyPos is the body operand's sig
// position, and inputs returns the per-invocation input carriers (generalised
// types, so the body does not constant-fold one call's values) in the order
// InvokeBody supplies them to the body.
type callableWord struct {
	bodyPos int
	inputs  func(args []Value) []Value
}

// callableWords is the closure-eligible set: pure, single-result code-body
// transforms whose handlers run their body via InvokeBody. Each body nets
// exactly one value per invocation (the closure unit's single declared
// return). Expanded only with the differential + full-corpus gates green.
var callableWords = map[string]callableWord{
	// each [body] data — body sees one element, returns the mapped value.
	"each": {0, func(a []Value) []Value {
		return []Value{NewCarrier(DataListElemTypeFromValue(a[1]))}
	}},
	// filter [body] data — body sees one element, returns a Boolean.
	"filter": {0, func(a []Value) []Value {
		return []Value{NewCarrier(DataListElemTypeFromValue(a[1]))}
	}},
	// fold [body] data init — body sees (accumulator, element). InvokeBody
	// supplies [acc, elem]; acc generalises to the init's type, or (no-init
	// 2-arg form) to the element type, since the accumulator starts as the
	// first element.
	"fold": {0, func(a []Value) []Value {
		elem := DataListElemTypeFromValue(a[1])
		if len(a) >= 3 {
			return []Value{NewCarrier(a[2].Parent), NewCarrier(elem)}
		}
		return []Value{NewCarrier(elem), NewCarrier(elem)}
	}},
	// scan [body] data — body sees (accumulator, element); the accumulator
	// starts as the first element, so both inputs carry the element type.
	"scan": {0, func(a []Value) []Value {
		e := DataListElemTypeFromValue(a[1])
		return []Value{NewCarrier(e), NewCarrier(e)}
	}},
	// do [body] — runs the body with no inputs and returns its single
	// residual value (a multi-value body nets != 1 and refuses to the island).
	"do": {0, func(a []Value) []Value {
		return []Value{}
	}},
	// with-decimal {opts} [body] — runs a 0-input body inside a scoped
	// BigDecimal rounding context (opts at sig 0, body at sig 1). Like `do`,
	// the body nets its single residual; the handler pushes the context around
	// InvokeBody so the VM-run body's BigDecimal ops read the override.
	"with-decimal": {1, func(a []Value) []Value {
		return []Value{}
	}},
}

// compileClosureBody compiles a code body (bodyToks) consuming the given input
// carriers into its own fn unit, returning (unitIndex, ok). The body is
// recorded into the CURRENT EmitState (r.Check.Emit) — callers run it once in
// a throwaway probe state to test compilability, then once in the real state.
// Mirrors the fn-def compile path (core_helpers.go): StartFnCompile arms the
// unit, AnalyseFnBody records the body under it, finish closes it. ok is false
// when the body refuses (StartFnCompile declined, or the analysis marked the
// state uncompilable).
// paramNames is the per-input name table: a NAMED slot (a lambda param,
// `([p] => …)`) binds the body's `p` to that input carrier in AnalyseFnBody;
// an empty name (the token-quotation form, `[body]`) leaves the input on the
// stack for the body to consume positionally. nil means all-unnamed.
func compileClosureBody(r *Registry, word string, bodyToks, inputs []Value, paramNames []string, captures []CapturedBinding, shape ClosureInShape, pos SrcPos) (int, bool) {
	es := r.Check.Emit
	declared := []*Type{TAny}
	if paramNames == nil {
		paramNames = make([]string, len(inputs)) // all unnamed: body reads inputs off the stack
	}
	name := word + "$body"
	key := FnAnalysisKey(name, inputs, captures, bodyToks)
	unit, finish, ok := es.StartFnCompile(key, name, inputs, declared, paramNames, captures, false, pos)
	if !ok {
		return -1, false
	}
	// Record the closure's input convention on the unit (consistent across a
	// memo hit: the key includes name+input types, which determine the shape).
	es.fnRecs[unit].inShape = shape
	if finish == nil {
		// Memo hit: the unit is already compiled in this state.
		return unit, es.active()
	}
	// Drop any summary a suspended (non-recording) analysis cached so the
	// body re-runs under the armed unit and records.
	delete(r.Check.FnSummaries, key)
	stk := AnalyseFnBody(r, name, paramNames, bodyToks, inputs, captures, declared)
	finish(stk)
	return unit, es.Compilable
}

// tryRecordClosure attempts to compile a code-body higher-order word's body to
// a closure unit and record a normal dispatch (the body operand lowering to
// OpPushClosure). Returns true on success. A body that does not compile leaves
// the REAL emit state untouched — the probe runs in a throwaway state — so the
// caller falls through to the island path.
func tryRecordClosure(r *Registry, word string, sig *Signature, args, outs []Value, pos SrcPos) bool {
	spec, ok := callableWords[word]
	if !ok {
		return false
	}
	es := r.Check.Emit
	if !es.active() || sig == nil || len(outs) != 1 || spec.bodyPos >= len(args) {
		return false
	}
	body := args[spec.bodyPos]

	// A lambda VALUE body (`filter ([p:Any] => …) data`): the afn's named
	// param binds to the WORD'S callback shape ({key,value} pair for list
	// filter, KeyVal for the map forms), not to its declared `Any`. Compile
	// the lambda body against that representative shape so `p.value`/`kv.v`
	// typechecks, then record the dispatch with the body as a closure the
	// handler drives through InvokeBody.
	if fd, isFn := body.Data.(FnDefInfo); isFn {
		return tryRecordLambdaClosure(r, word, spec, sig, args, &fd, outs, pos)
	}

	// A token-list body (`filter [body] data`): the body consumes its inputs
	// positionally off the stack.
	if !IsConcrete(body) {
		return false
	}
	bodyList, err := AsList(body)
	if err != nil || bodyList.IsNil() {
		return false
	}
	// A body with a flow-control sentinel (break/continue/return) targets an
	// enclosing loop the VM can't reach across the call boundary — keep it on
	// the whole-program fallback path.
	if bodyHasSentinel(body) {
		return false
	}
	inputs := spec.inputs(args)
	if inputs == nil {
		return false
	}
	bodyToks := bodyList.Slice()

	// Lexical captures: body words resolving to an ENCLOSING fn's binding
	// (a param or body-local of a fn currently being compiled) ride as the
	// closure's captures — resolved here in the enclosing scope, bound into
	// the body unit's trailing slots at invocation. A module/global ref is
	// not a capture (it bakes as a const in the body, or refuses the probe).
	captures := ComputeCaptures(r, &FnSig{Body: bodyToks})
	return recordClosureDispatch(r, word, spec, sig, args, bodyToks, inputs, nil, captures, ClosureInValue, outs, pos)
}

// tryRecordLambdaClosure compiles a higher-order word's LAMBDA argument
// (`([p] => …)`, an anonymous FnDefInfo) to a closure unit. The lambda's body
// is compiled with the word's per-callback input shape (lambdaCallbackInputs)
// bound to the lambda's NAMED params, so a body that destructures the entry
// (`p.value`, `kv.v`, `acc`+`kv.v`) typechecks. Returns false — leaving the
// refusal to stand — for a shape the word has no lambda convention for, an
// arity mismatch, a capturing lambda (deferred), or a body that does not
// compile.
func tryRecordLambdaClosure(r *Registry, word string, spec callableWord, sig *Signature, args []Value, fd *FnDefInfo, outs []Value, pos SrcPos) bool {
	lam, ok := fd.FirstOwnSig()
	if !ok || len(lam.Body) == 0 {
		return false
	}
	// An OVERLOADED fn value (more than one own signature) is dispatched by
	// MatchFnSig at runtime — FirstOwnSig is not necessarily the matched
	// overload, so compiling its body could run the wrong one. Refuse and let
	// the interpreter select the overload.
	own := 0
	for i := range fd.Signatures {
		if !fd.Signatures[i].Fallback {
			own++
		}
	}
	if own > 1 {
		return false
	}
	// A capturing lambda would need its captures resolved in this scope and
	// threaded onto the closure; the spec lambda rows are capture-free, so
	// defer that and keep a capturing lambda on the refusal path.
	if len(fd.Captured) > 0 {
		return false
	}
	if bodyToksHaveSentinel(lam.Body) {
		return false
	}
	inputs, shape, ok := lambdaCallbackInputs(r, word, args, lam)
	if !ok || len(lam.Params) != len(inputs) {
		return false
	}
	// The callback shape must satisfy each declared param TYPE — the same
	// membership the runtime MatchFnSig checks at dispatch. A param whose type
	// rejects the shape (e.g. `[p:String]` against filter's {key,value} pair)
	// makes the interpreter raise a callback error; compiling the body anyway
	// would silently keep the element instead. Refuse so the type check stands.
	for i := range lam.Params {
		// The callback shape must satisfy each declared param TYPE — the same
		// membership the runtime MatchFnSig checks at dispatch. A param whose
		// type rejects the shape (e.g. `[p:String]` against filter's
		// {key,value} pair, or `[kv:KeyVal]` against a list's plain pair) makes
		// the interpreter raise a callback error; compiling the body anyway
		// would silently keep the element. Refuse so the type check stands.
		pt := lam.Params[i].Type
		if pt == nil {
			continue
		}
		// A map-iteration ENTRY input is a KeyVal (a Map subtype) that the
		// carrier conservatively under-types as a plain Map — its concrete type
		// is not always resolvable here. Accept any Map-family param (Map,
		// KeyVal, Any, Node) and refuse a param that is neither a sub- nor a
		// super-type of Map (a scalar like String, or a sibling container),
		// which the runtime callback dispatch rejects.
		if shape == ClosureInKeyVal && inputs[i].Parent.ConformsTo(TMap) {
			if !pt.ConformsTo(TMap) && !TMap.ConformsTo(pt) {
				return false
			}
			continue
		}
		// Correctly-typed carriers (the {key,value} pair for the list forms, a
		// typed accumulator): the param must satisfy the carrier type, the same
		// membership the runtime MatchFnSig checks at dispatch. A mismatch
		// (`[p:String]` against filter's Map pair) makes the interpreter raise a
		// callback error while a compiled body would silently keep the element.
		if !sigTypeMatches(inputs[i], pt) {
			return false
		}
	}
	names := make([]string, len(lam.Params))
	for i := range lam.Params {
		names[i] = lam.Params[i].Name
	}
	return recordClosureDispatch(r, word, spec, sig, args, lam.Body, inputs, names, nil, shape, outs, pos)
}

// recordClosureDispatch is the shared tail of the token and lambda closure
// paths: it resolves the lexical captures, probe-compiles the body in a
// throwaway state (a refusal leaves the real program untouched), then
// real-compiles it and records the dispatch (the body operand lowering to
// OpPushClosure). paramNames is nil for the token form (stack-consumed inputs)
// and the lambda's param names for the lambda form.
func recordClosureDispatch(r *Registry, word string, spec callableWord, sig *Signature, args, bodyToks, inputs []Value, paramNames []string, captures []CapturedBinding, shape ClosureInShape, outs []Value, pos SrcPos) bool {
	capOps := make([]emitOperand, len(captures))
	for i, cb := range captures {
		op, ok := r.Check.Emit.resolveOperand(cb.Value)
		if !ok {
			return false // an unreachable capture — keep the island path
		}
		capOps[i] = op
	}

	// The body compile re-runs the body (the ReturnsFn pass already emitted
	// its diagnostics); drop any it re-emits so counts do not double.
	diagBase := len(r.Check.Diagnostics)
	defer r.Check.TruncateDiagnostics(diagBase)

	// PROBE: compile the body in a throwaway state so a refusal leaves the
	// real program untouched (graceful fall-through to the island).
	real := r.Check.Emit
	r.Check.Emit = NewEmitState()
	_, probeOk := compileClosureBody(r, word, bodyToks, inputs, paramNames, captures, shape, pos)
	r.Check.Emit = real
	if !probeOk {
		return false
	}

	// REAL: compile the body into the program (deterministic success after a
	// clean probe), then record the dispatch with the body as a closure.
	unit, realOk := compileClosureBody(r, word, bodyToks, inputs, paramNames, captures, shape, pos)
	if !realOk || unit < 0 {
		return false
	}
	return real.RecordClosureCall(word, sig, args, spec.bodyPos, unit, capOps, outs, pos)
}

// lambdaCallbackInputs returns the representative input carriers a higher-order
// word presents to a LAMBDA callback — the word's callback shape, which differs
// from the token-quotation form (spec.inputs) — plus the runtime ClosureInShape
// the driving handler reads to present each entry:
//
//   - filter over a LIST: one {key, value} pair Map (the element via `.value`).
//   - filter/each over a MAP: one KeyVal {k v i n} (the value via `.v`).
//   - fold (init form) / scan over a MAP: (accumulator, KeyVal).
//
// The carriers are GENERALISED (field types, not one call's values) so the body
// is compiled once for every entry. ok is false for a shape with no lambda
// convention (the caller then leaves the refusal to stand): a list each/fold,
// a no-init map fold, and for-each (whose check-mode output count does not match
// its 0-result runtime) all stay on the refusal path.
func lambdaCallbackInputs(r *Registry, word string, args []Value, _ *Signature) ([]Value, ClosureInShape, bool) {
	spec, ok := callableWords[word]
	if !ok || spec.bodyPos+1 >= len(args) {
		return nil, ClosureInValue, false
	}
	data := args[spec.bodyPos+1] // the data operand follows the body operand
	if !IsConcrete(data) {
		return nil, ClosureInValue, false
	}
	elem := DataListElemTypeFromValue(data)
	isMap := data.Parent.ConformsTo(TMap)
	isList := data.Parent.ConformsTo(TList)
	switch word {
	case "filter":
		switch {
		case isMap:
			return []Value{keyValCarrier(r, elem)}, ClosureInKeyVal, true
		case isList:
			return []Value{pairCarrier(elem)}, ClosureInValue, true
		}
	case "each":
		if isMap {
			return []Value{keyValCarrier(r, elem)}, ClosureInKeyVal, true
		}
	case "fold":
		// Init form only (`init fold (lambda) {m}` → args [lambda, map, init]):
		// the accumulator carries the seed's type, the entry rides as a KeyVal.
		if isMap && len(args) > spec.bodyPos+2 {
			acc := args[spec.bodyPos+2]
			return []Value{NewCarrier(acc.Parent), keyValCarrier(r, elem)}, ClosureInKeyVal, true
		}
	case "scan":
		// scan seeds the accumulator from the first value (no init operand): the
		// accumulator carries the value type, the entry rides as a KeyVal.
		if isMap {
			return []Value{NewCarrier(elem), keyValCarrier(r, elem)}, ClosureInKeyVal, true
		}
	}
	return nil, ClosureInValue, false
}

// pairCarrier builds a representative {key, value} pair Map carrier — the shape
// filter's list Function form hands its callback (key = the index, value = the
// element). Field VALUES are carriers (Integer key, elem value) so the compiled
// body reads field TYPES, never one call's concrete values.
func pairCarrier(elem *Type) Value {
	om := NewOrderedMap()
	om.Set("key", NewCarrier(TInteger))
	om.Set("value", NewCarrier(elem))
	return NewValueRaw(TMap, MapPayload{M: om})
}

// keyValCarrier builds a representative KeyVal {k v i n} carrier — the shape the
// map Function forms (filter/each/fold/scan over a map) hand their callback. The
// value field carries the map's common value type; k/i/n carry String/Integer/
// Integer. Tagged Node/Map/KeyVal when that type is registered (the language
// layer), else a plain Map carrier — either way the body's `kv.v`/`kv.i` reads
// resolve by ordinary dotted access.
func keyValCarrier(r *Registry, elem *Type) Value {
	om := NewOrderedMap()
	om.Set("k", NewCarrier(TString))
	om.Set("v", NewCarrier(elem))
	om.Set("i", NewCarrier(TInteger))
	om.Set("n", NewCarrier(TInteger))
	t := TMap
	if r != nil {
		if kv := r.Types.Lookup("Node/Map/KeyVal"); kv != nil {
			t = kv
		}
	}
	return NewValueRaw(t, MapPayload{M: om})
}

// bodyToksHaveSentinel reports whether a lambda body's token slice contains a
// flow-control sentinel (break/continue/return) — the token-slice form of
// bodyHasSentinel, for a lambda whose Body is already []Value.
func bodyToksHaveSentinel(toks []Value) bool {
	found := false
	WalkBodyWords(toks, func(w WordInfo, _ Value) {
		switch w.Name {
		case "break", "continue", "return":
			found = true
		}
	})
	return found
}
