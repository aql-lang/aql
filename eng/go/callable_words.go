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
		return []Value{NewCarrier(hofElemType(a[1]))}
	}},
	// filter [body] data — body sees one element, returns a Boolean.
	"filter": {0, func(a []Value) []Value {
		return []Value{NewCarrier(hofElemType(a[1]))}
	}},
	// fold [body] data init — body sees (accumulator, element). InvokeBody
	// supplies [acc, elem]; acc generalises to the init's type, or (no-init
	// 2-arg form) to the element type, since the accumulator starts as the
	// first element.
	"fold": {0, func(a []Value) []Value {
		elem := hofElemType(a[1])
		if len(a) >= 3 {
			return []Value{NewCarrier(a[2].Parent), NewCarrier(elem)}
		}
		return []Value{NewCarrier(elem), NewCarrier(elem)}
	}},
	// scan [body] data — body sees (accumulator, element); the accumulator
	// starts as the first element, so both inputs carry the element type.
	"scan": {0, func(a []Value) []Value {
		e := hofElemType(a[1])
		return []Value{NewCarrier(e), NewCarrier(e)}
	}},
	// do [body] — runs the body with no inputs and returns its single
	// residual value (a multi-value body nets != 1 and refuses to the island).
	"do": {0, func(a []Value) []Value {
		return []Value{}
	}},
}

// hofElemType is the body input carrier for a higher-order word's data
// receiver: a map's VALUE type when the receiver is a map, else the list
// element type. A map-receiver quotation body sees each value, exactly as a
// list body sees each element.
func hofElemType(data Value) *Type {
	if data.Parent != nil && data.Parent.ConformsTo(TMap) {
		return DataMapValueTypeFromValue(data)
	}
	return DataListElemTypeFromValue(data)
}

// compileClosureBody compiles a code body (bodyToks) consuming the given input
// carriers into its own fn unit, returning (unitIndex, ok). The body is
// recorded into the CURRENT EmitState (r.Check.Emit) — callers run it once in
// a throwaway probe state to test compilability, then once in the real state.
// Mirrors the fn-def compile path (core_helpers.go): StartFnCompile arms the
// unit, AnalyseFnBody records the body under it, finish closes it. ok is false
// when the body refuses (StartFnCompile declined, or the analysis marked the
// state uncompilable).
func compileClosureBody(r *Registry, word string, bodyToks, inputs []Value, paramNames []string, captures []CapturedBinding, pos SrcPos) (int, bool) {
	es := r.Check.Emit
	declared := []*Type{TAny}
	name := word + "$body"
	key := FnAnalysisKey(name, inputs, captures, bodyToks)
	unit, finish, ok := es.StartFnCompile(key, name, inputs, declared, paramNames, captures, false, pos)
	if !ok {
		return -1, false
	}
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

	// Quotation bodies read their inputs off the stack — all params unnamed.
	paramNames := make([]string, len(inputs))

	// PROBE: compile the body in a throwaway state so a refusal leaves the
	// real program untouched (graceful fall-through to the island).
	real := r.Check.Emit
	r.Check.Emit = NewEmitState()
	_, probeOk := compileClosureBody(r, word, bodyToks, inputs, paramNames, captures, pos)
	r.Check.Emit = real
	if !probeOk {
		return false
	}

	// REAL: compile the body into the program (deterministic success after a
	// clean probe), then record the dispatch with the body as a closure.
	unit, realOk := compileClosureBody(r, word, bodyToks, inputs, paramNames, captures, pos)
	if !realOk || unit < 0 {
		return false
	}
	return real.RecordClosureCall(word, sig, args, spec.bodyPos, unit, capOps, outs, pos)
}

// tryRecordLambdaClosure compiles a higher-order word handed a LAMBDA VALUE
// (an anonymous, single-sig FnDefInfo built by `=>`/afn) to a closure unit,
// recording a native dispatch instead of refusing at RecordCall's fn-value
// guards. Scoped (roadmap item 5a-1) to `filter (lambda) [list]`: the handler
// hands the lambda a {key:index, value:elem} pair Map, so the closure's single
// named param binds to that pair-Map carrier. Other receivers/words stay
// refused. Returns true on success, leaving the real emit state untouched on
// any refusal (the probe runs in a throwaway state).
func tryRecordLambdaClosure(r *Registry, word string, sig *Signature, args, outs []Value, pos SrcPos) bool {
	es := r.Check.Emit
	if !es.active() || sig == nil || len(outs) != 1 {
		return false
	}
	// Only filter over a concrete LIST receiver (the pair-Map input shape).
	if word != "filter" || len(args) < 2 {
		return false
	}
	if !IsConcrete(args[1]) || !args[1].Parent.ConformsTo(TList) {
		return false
	}
	lambda := args[0]
	fnDef, ok := lambda.Data.(FnDefInfo)
	if !ok || !fnDef.Anonymous || len(fnDef.Signatures) != 1 {
		return false // not a single-sig anonymous lambda — keep it refused
	}
	fs := fnDef.Signatures[0]
	if len(fs.Params) != 1 || len(fs.Body) == 0 {
		return false // the predicate is a single-param body
	}
	if bodyHasSentinel(NewList(fs.Body)) {
		return false // break/continue/return targets an unreachable loop
	}
	paramNames := []string{fs.Params[0].Name}
	bodyToks := fs.Body

	// The handler builds a {key:index, value:elem} pair Map per element
	// (filter.go). Bind the lambda's param to a matching pair-Map carrier so
	// `p.value` lowers against the element type.
	pair := NewOrderedMap()
	pair.Set("key", NewCarrier(TInteger))
	pair.Set("value", NewCarrier(DataListElemTypeFromValue(args[1])))
	inputs := []Value{NewMap(pair)}

	// Lexical captures ride from the lambda's construction-time snapshot (the
	// correct enclosing baseline), resolved in the current emit scope.
	captures := fnDef.Captured
	capOps := make([]emitOperand, len(captures))
	for i, cb := range captures {
		op, ok := es.resolveOperand(cb.Value)
		if !ok {
			return false // an unreachable capture — keep it refused
		}
		capOps[i] = op
	}

	diagBase := len(r.Check.Diagnostics)
	defer r.Check.TruncateDiagnostics(diagBase)

	// PROBE in a throwaway state, then compile for real.
	real := r.Check.Emit
	r.Check.Emit = NewEmitState()
	_, probeOk := compileClosureBody(r, word, bodyToks, inputs, paramNames, captures, pos)
	r.Check.Emit = real
	if !probeOk {
		return false
	}
	unit, realOk := compileClosureBody(r, word, bodyToks, inputs, paramNames, captures, pos)
	if !realOk || unit < 0 {
		return false
	}
	return real.RecordClosureCall(word, sig, args, 0, unit, capOps, outs, pos)
}
