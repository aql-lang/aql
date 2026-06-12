package eng

// The bytecode recording pass — Stage 1 of design/aql-bytecode-plan.0.md.
//
// The compiler is the carrier checker with a recording side effect:
// when CheckState.Emit is set, every check-mode dispatch that flows
// through carrierResults records a call event with full operand
// provenance, and Finalize linearises the event trace into a Program.
//
// Site taxonomy (aql-bytecode-readiness.0.md gap 1) — every dispatch
// is classified from the first commit:
//
//   - mono                — single checker-selected signature; compiles.
//   - poly (partitioned)  — a strict disjunct straddles signatures;
//     Stage 1 marks the program uncompilable, later stages emit
//     CALL_NATIVE_POLY here.
//   - dynamic             — a dynamic carrier reached the site; fallback
//     territory.
//   - meta                — RunInCheckMode / code-body / user-fn /
//     recovered dispatches; compile-time-only or fallback.
//
// Operand provenance rides Value.ID: NewValueRaw mints a unique ID
// per value and toCarrier/copies preserve it, so a dispatch arg is
// (a) a recorded output of an earlier call, (b) a literal — concrete
// at the dispatch, or a stripped top-level literal whose original
// was saved by RecordStrip — or (c) unknown, which marks the program
// uncompilable rather than guessing.

// Site classes for SiteCounts.
const (
	SiteMono    = "mono"
	SitePoly    = "poly"
	SiteDynamic = "dynamic"
	SiteMeta    = "meta"
)

type emitOperand struct {
	constIdx int // >=0: Consts index
	fromCall int // >=0: producing call event index
}

type emitCall struct {
	word  string
	sig   *Signature
	ops   []emitOperand
	outID string
	pos   SrcPos
}

// EmitState is the recording side of the compile pass. Set
// CheckState.Emit to a NewEmitState before a check run; call
// Finalize afterwards. All methods are nil-receiver-safe so hook
// sites need no guards.
type EmitState struct {
	// Compilable latches false at the first construct Stage 1 cannot
	// lower; Reason names the first offender.
	Compilable bool
	Reason     string
	// SiteCounts tallies dispatches per site class while recording is
	// active (counting stops once the program is marked
	// uncompilable, with the rest of the recording).
	SiteCounts map[string]int

	suspended  int
	calls      []emitCall
	producedBy map[string]int // value ID → call index
	consts     []Value
	constIdx   map[string]int   // CanonValue → Consts index
	origByID   map[string]Value // stripped literal ID → original value
}

// NewEmitState returns a fresh recording state.
func NewEmitState() *EmitState {
	return &EmitState{
		Compilable: true,
		SiteCounts: map[string]int{},
		producedBy: map[string]int{},
		constIdx:   map[string]int{},
		origByID:   map[string]Value{},
	}
}

func (es *EmitState) active() bool {
	return es != nil && es.Compilable && es.suspended == 0
}

// MarkUncompilable latches the program uncompilable, keeping the
// FIRST reason (later marks are consequences of the first).
func (es *EmitState) MarkUncompilable(reason string) {
	if es == nil || !es.Compilable {
		return
	}
	es.Compilable = false
	es.Reason = reason
}

// Suspend pauses recording for the duration of a nested body
// analysis (branch bodies, fn bodies, higher-order bodies run their
// own sub-engines over the shared registry; their dispatches are not
// part of THIS program's straight line). Returns the resume func.
func (es *EmitState) Suspend() func() {
	if es == nil {
		return func() {}
	}
	es.suspended++
	return func() { es.suspended-- }
}

// RecordStrip remembers the original concrete value behind a
// top-level literal that StripToCarriers reduced to a carrier — the
// ID is preserved by the strip, so a later dispatch arg with this ID
// is that literal.
func (es *EmitState) RecordStrip(orig, stripped Value) {
	if es == nil || es.suspended > 0 {
		return
	}
	if stripped.Carrier && !orig.Carrier && orig.ID != "" && orig.ID == stripped.ID {
		es.origByID[orig.ID] = orig
	}
}

// RecordPoly classifies a partitioned (per-alternative) dispatch.
// Stage 1 cannot lower it; later stages emit CALL_NATIVE_POLY.
func (es *EmitState) RecordPoly(word string) {
	if !es.active() {
		return
	}
	es.SiteCounts[SitePoly]++
	es.MarkUncompilable("polymorphic dispatch at " + word)
}

// RecordCall records one resolved dispatch. args are in signature
// order (position 0 = top of stack); outs are the carrier results.
func (es *EmitState) RecordCall(word string, sig *Signature, args, outs []Value, pos SrcPos) {
	if !es.active() {
		return
	}
	switch {
	case sig == nil:
		es.MarkUncompilable("dispatch without a signature at " + word)
		return
	case word == "":
		// Anonymous / fn-value dispatch (usurp wrappers, F4 value
		// calls): the callee is a runtime value, Stage 3 territory.
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("anonymous function dispatch (Stage 3)")
		return
	case sig.RunInCheckMode:
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("compile-time word " + word)
		return
	case sig.FnFrame != nil:
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("user fn call " + word + " (Stage 3)")
		return
	case sig.FullStack:
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("full-stack word " + word)
		return
	case len(sig.NoEvalArgs) > 0:
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("code-body word " + word + " (Stage 2)")
		return
	case len(sig.QuoteArgs) > 0:
		// Implicit-quote operands (usurp, force-arity, ref-family):
		// dispatch-manipulating meta words whose results the engine
		// re-steps.
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("quoted-operand word " + word)
		return
	case len(sig.TypeArgs) > 0:
		// Type-literal operands (make, is, convert, tand, …): the
		// operand must be the CANONICAL registry type node — a
		// constant-pool copy goes stale against later behaviour/field
		// installs (eng/go/CLAUDE.md, Canonical *Type Pointers).
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable("type-operand word " + word + " (Stage 4)")
		return
	case anyDynamicCarrier(args):
		es.SiteCounts[SiteDynamic]++
		es.MarkUncompilable("dynamic input at " + word)
		return
	case anyDynamicCarrier(outs):
		// Dynamic outputs mean the checker could not type the word
		// (missing annotations, opaque wrappers like a def-bound
		// usurp value): the recorded signature is a best guess, not a
		// proof — don't bake it in.
		es.SiteCounts[SiteDynamic]++
		es.MarkUncompilable("unannotated or opaque word " + word)
		return
	case len(outs) != 1:
		es.SiteCounts[SiteMeta]++
		es.MarkUncompilable(word + " returns " + itoaSmall(len(outs)) + " values (Stage 1 lowers single-result calls)")
		return
	}
	// Function-valued operands mean a fn-invoking word (apply, usurp,
	// higher-order forms): their handlers return values the ENGINE
	// re-steps on the tape, which a VM cannot honour. Stage 3
	// territory.
	for _, t := range sig.Args {
		if t != nil && (t.ConformsTo(TFunction) || t.ConformsTo(TFnDef)) {
			es.SiteCounts[SiteMeta]++
			es.MarkUncompilable("function-valued operand at " + word + " (Stage 3)")
			return
		}
	}
	for _, a := range args {
		if _, ok := a.Data.(FnDefInfo); ok {
			es.SiteCounts[SiteMeta]++
			es.MarkUncompilable("function value reaches " + word + " (Stage 3)")
			return
		}
	}
	ops := make([]emitOperand, len(args))
	for i, a := range args {
		ops[i] = emitOperand{constIdx: -1, fromCall: -1}
		if idx, ok := es.producedBy[a.ID]; ok {
			ops[i].fromCall = idx
			continue
		}
		lit := a
		if !IsConcrete(a) {
			orig, ok := es.origByID[a.ID]
			if !ok {
				es.MarkUncompilable("operand of unknown provenance at " + word)
				return
			}
			lit = orig
		}
		if !isInertConst(lit) {
			es.MarkUncompilable("operand not statically materialisable at " + word)
			return
		}
		ops[i].constIdx = es.intern(lit)
	}
	es.SiteCounts[SiteMono]++
	es.calls = append(es.calls, emitCall{word: word, sig: sig, ops: ops, outID: outs[0].ID, pos: pos})
	es.producedBy[outs[0].ID] = len(es.calls) - 1
}

// intern pools a constant by canonical form. Compounds (lists,
// maps) are NEVER pooled: `eq` on compounds compares by identity
// (compare-restrict), so two source literals must stay two constants
// with their two distinct IDs — pooling them made `[1] eq [1]` true
// under the VM where the interpreter says false (the report's
// gotcha #13, caught by the differential gate).
func (es *EmitState) intern(v Value) int {
	if v.Parent.Equal(TList) || v.Parent.Equal(TMap) {
		es.consts = append(es.consts, v)
		return len(es.consts) - 1
	}
	key := CanonValue(v)
	if i, ok := es.constIdx[key]; ok {
		return i
	}
	es.consts = append(es.consts, v)
	es.constIdx[key] = len(es.consts) - 1
	return len(es.consts) - 1
}

// isInertConst reports whether v can live in a Program's constant
// pool: a fully concrete value whose payload is PLAIN DATA. The rule
// is a whitelist — scalars, temporal values, and lists/maps of the
// same — because anything else is engine-coupled in some way: a
// check-mode carrier must not be materialised; structural tokens
// (Word, Splice, Reach, ParenExpr) would be expanded or re-stepped
// by the engine; type literals and type bodies (class, surface,
// record) are registry nodes whose by-value copies go stale against
// the canonical pointer (eng/go/CLAUDE.md, Canonical *Type
// Pointers); function values are Stage 3.
func isInertConst(v Value) bool {
	if v.Carrier || v.Dynamic || IsBareTypeNode(v) {
		return false
	}
	switch d := v.Data.(type) {
	case IntPayload, FloatPayload, StrPayload, BoolPayload, AtomPayload,
		PathPayload, NonePayload, BigIntPayload, DecimalPayload,
		TimePayload, DurationPayload, TimezonePayload:
		return true
	case ListPayload:
		for _, e := range d.Elems {
			if !isInertConst(e) {
				return false
			}
		}
		return true
	case MapPayload:
		if d.M == nil {
			return false
		}
		for _, k := range d.M.Keys() {
			mv, _ := d.M.Get(k)
			if !isInertConst(mv) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// Finalize linearises the recorded events into a Program. ok=false
// (with reason) when the source was marked uncompilable or the
// linear stack discipline doesn't hold under Stage 1's operand
// shapes (all-const; one prior result plus consts; two prior
// results, optionally SWAPped).
// residual is the check run's final carrier stack: the program's
// declared result. Call-produced entries must match the simulated
// stack in order; literal entries may only sit ABOVE the last
// call-produced entry and are pushed at the end (a bare literal
// program like `5` compiles to one PUSH_CONST).
func (es *EmitState) Finalize(residual []Value) (*Program, string, bool) {
	if es == nil {
		return nil, "no emit state", false
	}
	if !es.Compilable {
		return nil, es.Reason, false
	}
	p := &Program{Consts: es.consts}
	var vm []int // simulated stack of producing call indices; -1 = const
	vmCall := func(depth int) int { return vm[len(vm)-1-depth] }
	sigIdx := map[*Signature]int{}
	depth, maxDepth := 0, 0
	emit := func(op Opcode, arg int, pos SrcPos) {
		p.Code = append(p.Code, Instr{Op: op, Arg: int32(arg)})
		p.Debug = append(p.Debug, pos)
		switch op {
		case OpPushConst:
			depth++
		case OpCallNative:
			// arity-1 pops + 1 push applied by caller
		}
		if depth > maxDepth {
			maxDepth = depth
		}
	}
	for ci, c := range es.calls {
		n := len(c.ops)
		// Partition operands: prior results must already sit on the
		// simulated stack; consts get pushed now.
		results := []int{} // positions i (sig order) sourced from calls
		for i, op := range c.ops {
			if op.fromCall >= 0 {
				results = append(results, i)
			}
		}
		switch len(results) {
		case 0:
			// Push consts deepest-first so sig position 0 lands on top.
			for i := n - 1; i >= 0; i-- {
				emit(OpPushConst, c.ops[i].constIdx, c.pos)
				vm = append(vm, -1)
			}
		case 1:
			ri := results[0]
			if len(vm) == 0 || vmCall(0) != c.ops[ri].fromCall {
				return nil, "stack discipline: result operand of " + c.word + " is not on top", false
			}
			// The prior result is on top. Push the const operands
			// deepest-first; they land ABOVE the result, so when the
			// result must be sig position 0 (top) a final SWAP
			// restores the layout — only the 2-arg shape is fixable.
			for i := n - 1; i >= 0; i-- {
				if i == ri {
					continue
				}
				emit(OpPushConst, c.ops[i].constIdx, c.pos)
				vm = append(vm, -1)
			}
			if ri == 0 && n == 2 {
				emit(OpSwap, 0, c.pos)
				vm[len(vm)-1], vm[len(vm)-2] = vm[len(vm)-2], vm[len(vm)-1]
			} else if ri != n-1 && n > 1 {
				return nil, "operand shape at " + c.word + " needs reordering beyond Stage 1", false
			}
		case 2:
			if n != 2 {
				return nil, "operand shape at " + c.word + " beyond Stage 1", false
			}
			if len(vm) < 2 {
				return nil, "stack discipline underflow at " + c.word, false
			}
			top, below := vmCall(0), vmCall(1)
			switch {
			case top == c.ops[0].fromCall && below == c.ops[1].fromCall:
				// already in layout
			case top == c.ops[1].fromCall && below == c.ops[0].fromCall:
				emit(OpSwap, 0, c.pos)
				vm[len(vm)-1], vm[len(vm)-2] = vm[len(vm)-2], vm[len(vm)-1]
			default:
				return nil, "stack discipline: operands of " + c.word + " not adjacent on top", false
			}
		default:
			return nil, "operand shape at " + c.word + " beyond Stage 1", false
		}
		// The call: pop n, push the result.
		si, ok := sigIdx[c.sig]
		if !ok {
			p.Sigs = append(p.Sigs, SigRef{Word: c.word, Sig: c.sig})
			si = len(p.Sigs) - 1
			sigIdx[c.sig] = si
		}
		emit(OpCallNative, si, c.pos)
		vm = vm[:len(vm)-n]
		vm = append(vm, ci)
		depth = depth - n + 1
		if depth > maxDepth {
			maxDepth = depth
		}
	}
	// Residual reconciliation: the program must end with exactly the
	// check run's residual stack.
	lastPos := SrcPos{}
	if len(es.calls) > 0 {
		lastPos = es.calls[len(es.calls)-1].pos
	}
	vi := 0
	constTail := []int{}
	for _, rv := range residual {
		if idx, ok := es.producedBy[rv.ID]; ok {
			if len(constTail) > 0 {
				return nil, "residual shape beyond Stage 1 (call result above a literal)", false
			}
			if vi >= len(vm) || vm[vi] != idx {
				return nil, "residual shape beyond Stage 1 (call results reordered)", false
			}
			vi++
			continue
		}
		lit := rv
		if !IsConcrete(rv) {
			orig, ok := es.origByID[rv.ID]
			if !ok {
				return nil, "residual value of unknown provenance", false
			}
			lit = orig
		}
		if !isInertConst(lit) {
			return nil, "residual value not statically materialisable", false
		}
		constTail = append(constTail, es.intern(lit))
	}
	if vi != len(vm) {
		return nil, "residual shape beyond Stage 1 (unconsumed call results)", false
	}
	p.Consts = es.consts // interning may have grown during reconciliation
	for _, k := range constTail {
		emit(OpPushConst, k, lastPos)
	}
	if d := len(vm) + len(constTail); d > maxDepth {
		maxDepth = d
	}
	p.MaxStack = maxDepth
	return p, "", true
}

func itoaSmall(n int) string {
	if n >= 0 && n < 10 {
		return string(rune('0' + n))
	}
	return "many"
}
