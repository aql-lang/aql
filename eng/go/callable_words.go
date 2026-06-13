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
	// supplies [acc, elem]; acc generalises to the init's type.
	"fold": {0, func(a []Value) []Value {
		if len(a) < 3 {
			return nil
		}
		return []Value{NewCarrier(a[2].Parent), NewCarrier(DataListElemTypeFromValue(a[1]))}
	}},
	// scan [body] data — body sees (accumulator, element); the accumulator
	// starts as the first element, so both inputs carry the element type.
	"scan": {0, func(a []Value) []Value {
		e := DataListElemTypeFromValue(a[1])
		return []Value{NewCarrier(e), NewCarrier(e)}
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
func compileClosureBody(r *Registry, word string, bodyToks, inputs []Value, pos SrcPos) (int, bool) {
	es := r.Check.Emit
	declared := []*Type{TAny}
	paramNames := make([]string, len(inputs)) // all unnamed: body reads inputs off the stack
	name := word + "$body"
	key := FnAnalysisKey(name, inputs, nil, bodyToks)
	unit, finish, ok := es.StartFnCompile(key, name, inputs, declared, paramNames, nil, false)
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
	stk := AnalyseFnBody(r, name, paramNames, bodyToks, inputs, nil, declared)
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
	// A body referencing a check-time def / capture cannot bake into a unit
	// the VM resolves; keep those on the island path for now.
	if !bodyFreeForFallback(r, body) {
		return false
	}
	inputs := spec.inputs(args)
	if inputs == nil {
		return false
	}
	bodyToks := bodyList.Slice()

	// The body compile re-runs the body (the ReturnsFn pass already emitted
	// its diagnostics); drop any it re-emits so counts do not double.
	diagBase := len(r.Check.Diagnostics)
	defer r.Check.TruncateDiagnostics(diagBase)

	// PROBE: compile the body in a throwaway state so a refusal leaves the
	// real program untouched (graceful fall-through to the island).
	real := r.Check.Emit
	r.Check.Emit = NewEmitState()
	_, probeOk := compileClosureBody(r, word, bodyToks, inputs, pos)
	r.Check.Emit = real
	if !probeOk {
		return false
	}

	// REAL: compile the body into the program (deterministic success after a
	// clean probe), then record the dispatch with the body as a closure.
	unit, realOk := compileClosureBody(r, word, bodyToks, inputs, pos)
	if !realOk || unit < 0 {
		return false
	}
	return real.RecordClosureCall(word, sig, args, spec.bodyPos, unit, outs, pos)
}
