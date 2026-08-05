package specfix

// Control-flow fixtures: a minimal `if` (3-arg) and `for` (count form)
// that orchestrate the SAME eng mechanisms the production handlers in
// basic/go/native_control.go do — the mark/move continuation tokens at
// run time, and the branch/loop capture + record protocol
// (ArmBranchCapture / TakeFragment / RecordBranch, ArmLoopCapture /
// AnalyseLoopBody / RecordLoop) under an active recorder — so the
// standalone lanes drive the kernel's branch/loop analysis, lowering,
// and VM jump/loop opcodes (design/ENG-COVERAGE-PARITY.0.md stage-4
// plan, step 1). Deliberately narrower than basic's words: no static-if
// reduction, no clause lists, no computed arms (those mark the program
// uncompilable rather than model something the runtime would not do),
// and only the integer-count `for`.

import (
	eng "github.com/boru-lang/boru/eng/go"
)

// fixSpliceArg mirrors basic's spliceArg: a plain concrete list is a
// code body — wrap it in parens so the engine evaluates it inline —
// and anything else is a value used as-is.
func fixSpliceArg(v eng.Value) []eng.Value {
	if fixIsCodeBody(v) {
		elems, _ := eng.AsList(v)
		result := make([]eng.Value, 0, elems.Len()+2)
		result = append(result, eng.NewOpenParen())
		for i := 0; i < elems.Len(); i++ {
			result = append(result, elems.Get(i))
		}
		result = append(result, eng.NewCloseParen())
		return result
	}
	return []eng.Value{v}
}

func fixIsCodeBody(v eng.Value) bool {
	return v.Parent.Equal(eng.TList) && v.Data != nil && !eng.IsTypedList(v) && !eng.IsTableType(v)
}

// fixIfMarkMoveTokens builds the lazy-condition token stream: mark +
// condition body + move-if carrying the two branch continuations.
func fixIfMarkMoveTokens(cond eng.Value, thenBranch, elseBranch []eng.Value) ([]eng.Value, bool) {
	if !fixIsCodeBody(cond) {
		return nil, false
	}
	lst, _ := eng.AsList(cond)
	condSlice := lst.Slice()
	id := eng.NextMarkID()
	tokens := make([]eng.Value, 0, len(condSlice)+2)
	tokens = append(tokens, eng.NewMark(id, condSlice...))
	tokens = append(tokens, condSlice...)
	tokens = append(tokens, eng.NewMoveIf(id, "if", &eng.IfCont{
		Then: thenBranch,
		Else: elseBranch,
	}))
	return tokens, true
}

func fixIf3Handler(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
	cond := args[0]
	thenBranch := fixSpliceArg(args[1])
	elseBranch := fixSpliceArg(args[2])
	if tokens, ok := fixIfMarkMoveTokens(cond, thenBranch, elseBranch); ok {
		return tokens, nil
	}
	if eng.CoerceBoolean(cond) {
		return thenBranch, nil
	}
	return elseBranch, nil
}

// fixCondFragment captures the condition body as its own fragment when
// emitting, so the lowering runs it inline before JMP_IF_FALSE.
func fixCondFragment(r *eng.Registry, cond eng.Value) (*eng.EmitFragment, []eng.Value) {
	es := r.Check.Recorder()
	if !es.Armed() || !eng.IsConcrete(cond) || !cond.Parent.ConformsTo(eng.TList) {
		return nil, nil
	}
	es.ArmBranchCapture()
	stk, _ := eng.RunCarrierCondBody(r, cond)
	return es.TakeFragment(), stk
}

// fixIf3ReturnsFn is the check/emit model: the literal-condition
// reduction, per-arm body capture under guard narrowing, the carrier
// join, and the BranchRecord — the core of basic's if3ReturnsFn with
// the plain-check-only optimisations left out.
func fixIf3ReturnsFn(args []eng.Value, r *eng.Registry) []eng.Value {
	es := r.Check
	if lit, ok := eng.LiteralCondValue(args[0]); ok {
		branch := "else"
		if !lit {
			branch = "then"
		}
		r.Check.AddDiagnostic(eng.CheckDiagnostic{
			Code:   "unreachable_branch",
			Detail: "if condition is a constant " + eng.BoolWord(lit) + "; " + branch + "-branch is unreachable",
			Word:   "if",
			Row:    r.Check.CurCallPos.Row,
			Col:    r.Check.CurCallPos.Col,
		})
		var stk []eng.Value
		var defs map[string]eng.Value
		if lit {
			restore := eng.ApplyGuardNarrowing(r, args[0])
			es.Recorder().ArmBranchCapture()
			stk, defs = eng.RunCarrierBodyWithDefs(r, args[1])
			restore()
			eng.InstallJoinedDefs(r, defs, nil)
		} else {
			restore := eng.ApplyComplementNarrowing(r, args[0])
			es.Recorder().ArmBranchCapture()
			stk, defs = eng.RunCarrierBodyWithDefs(r, args[2])
			restore()
			eng.InstallJoinedDefs(r, nil, defs)
		}
		frag := es.Recorder().TakeFragment()
		if len(stk) == 0 {
			es.Recorder().MarkUncompilable("fixture if: constant-cond branch produces no value")
			return nil
		}
		out := stk[len(stk)-1]
		taken := lit
		es.Recorder().RecordBranch(eng.BranchRecord{
			ConstCond: &taken, HasElse: true,
			Then: frag, ThenStk: stk, Out: out, Pos: args[0].Pos(),
		})
		return []eng.Value{out}
	}

	condFrag, condStk := fixCondFragment(r, args[0])

	arm := func(v eng.Value, narrow func() func()) (frag *eng.EmitFragment, stk []eng.Value, defs map[string]eng.Value, value *eng.Value) {
		if fixIsCodeBody(v) {
			restore := narrow()
			es.Recorder().ArmBranchCapture()
			stk, defs = eng.RunCarrierBodyWithDefs(r, v)
			frag = es.Recorder().TakeFragment()
			restore()
			return frag, stk, defs, nil
		}
		if es.Recorder().Active() && !eng.IsConcrete(v) && v.Parent != nil && v.Parent.ConformsTo(eng.TList) {
			// A COMPUTED list arm is the interpreter's spliced code body;
			// the fixture does not model it — refuse the compile.
			es.Recorder().MarkUncompilable("fixture if: computed list arm")
		}
		vv := v
		return nil, []eng.Value{v}, nil, &vv
	}
	thenFrag, thenStk, thenDefs, thenValue := arm(args[1], func() func() { return eng.ApplyGuardNarrowing(r, args[0]) })
	elseFrag, elseStk, elseDefs, elseValue := arm(args[2], func() func() { return eng.ApplyComplementNarrowing(r, args[0]) })

	eng.InstallJoinedDefs(r, thenDefs, elseDefs)
	joined := eng.JoinCarrierStacks(thenStk, elseStk)
	if len(joined) == 0 {
		out := eng.NewCarrier(eng.TNone)
		es.Recorder().RecordBranch(eng.BranchRecord{
			Cond: args[0], CondFrag: condFrag, CondStk: condStk, HasElse: true,
			Then: thenFrag, Els: elseFrag, ThenStk: thenStk, ElsStk: elseStk,
			ThenValue: thenValue, ElsValue: elseValue, Out: out, Pos: args[0].Pos(),
		})
		if !es.Recorder().Active() {
			return nil
		}
		return []eng.Value{out}
	}
	out := joined[len(joined)-1]
	es.Recorder().RecordBranch(eng.BranchRecord{
		Cond: args[0], CondFrag: condFrag, CondStk: condStk, HasElse: true,
		Then: thenFrag, Els: elseFrag, ThenStk: thenStk, ElsStk: elseStk,
		ThenValue: thenValue, ElsValue: elseValue, Out: out, Pos: args[0].Pos(),
	})
	return []eng.Value{out}
}

// fixRunForLoop mirrors basic's RunForLoop for the count form: install
// the iterator, build mark + body + move-cont tokens; the engine's
// stepMoveCont drives the remaining iterations through the ForCont.
func fixRunForLoop(r *eng.Registry, start, end, step int64, iterName string, body eng.Value) ([]eng.Value, error) {
	if step > 0 && start >= end {
		return nil, nil
	}
	if !eng.IsConcrete(body) { //covergate:allow both callers pass a dispatch-matched List argument and handlers never run on carriers; a bare List literal cannot bind the sig slot
		return nil, &eng.BoruError{Code: "for_error", Detail: "for: body must be a concrete list, got type literal"}
	}
	lst, _ := eng.AsList(body)
	bodySlice := lst.Slice()

	eng.InstallDef(r, iterName, eng.NewInteger(start))

	bodyCopy := make([]eng.Value, len(bodySlice))
	copy(bodyCopy, bodySlice)
	cont := &eng.ForCont{
		Registry: r,
		IterName: iterName,
		Current:  start,
		End:      end,
		Step:     step,
		Body:     bodyCopy,
	}

	id := eng.NextMarkID()
	tokens := make([]eng.Value, 0, len(bodySlice)+2)
	tokens = append(tokens, eng.NewMark(id, bodySlice...))
	bodyTokens := make([]eng.Value, len(bodySlice))
	copy(bodyTokens, bodySlice)
	tokens = append(tokens, bodyTokens...)
	tokens = append(tokens, eng.NewMoveCont(id, "for loop", cont))
	return tokens, nil
}

func fixForCountHandler(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, r *eng.Registry) ([]eng.Value, error) {
	n, err := args[0].AsConcreteInteger()
	if err != nil { //covergate:allow the count is a dispatch-matched Integer argument and handlers never run on carriers; a bare Integer literal cannot bind the sig slot
		return nil, &eng.BoruError{Code: "for_error", Detail: "for: count must be a concrete Integer"}
	}
	return fixRunForLoop(r, 0, n, 1, "i", args[1])
}

// fixForReturnsFn is the count-form loop model: fixed-point body
// analysis with the iterator as a typed carrier, the statically-zero
// prune, and the RecordLoop event with its static region size.
func fixForReturnsFn(args []eng.Value, r *eng.Registry) []eng.Value {
	body := args[1]
	iter := eng.NewCarrier(eng.TInteger)
	es := r.Check.Recorder()

	cv := args[0]
	staticCount := int64(-1)
	if eng.IsConcrete(cv) && cv.Parent.ConformsTo(eng.TInteger) {
		if n, err := eng.AsInteger(cv); err == nil {
			staticCount = n
			if n <= 0 {
				return []eng.Value{}
			}
		}
	}

	startV, endV, stepV := eng.NewInteger(0), cv, eng.NewInteger(1)
	lowerable := cv.Parent.ConformsTo(eng.TInteger)
	if lowerable {
		es.ArmLoopCapture()
	}
	provenTrips := staticCount >= 1
	stk := eng.AnalyseLoopBody(r, body, []string{"i"}, []eng.Value{iter}, provenTrips)
	out := eng.NewCarrier(eng.TList)
	if len(stk) > 0 {
		top := stk[len(stk)-1]
		if eng.IsDisjunct(top) {
			out = eng.NewCarrierTypedListValue(top)
		} else {
			out = eng.NewCarrierTypedList(top.Parent)
		}
	}
	if lowerable {
		regionN := 0
		if staticCount >= 1 && len(stk) > 0 {
			regionN = int(staticCount) * len(stk)
		}
		frag := es.TakeFragment()
		es.RecordLoop(startV, endV, stepV, frag, stk, iter.ID, out, regionN, args[0].Pos())
	}
	if len(stk) == 0 && (!es.Active() || !lowerable) {
		return []eng.Value{}
	}
	// Plain check: a statically-counted loop leaves the SPREAD of its
	// per-iteration residual, exactly like the runtime.
	const spreadCap = 256
	if !es.Active() && staticCount >= 0 && len(stk) > 0 && staticCount*int64(len(stk)) <= spreadCap {
		spread := make([]eng.Value, 0, int(staticCount)*len(stk))
		for i := int64(0); i < staticCount; i++ {
			spread = append(spread, stk...)
		}
		return spread
	}
	return []eng.Value{out}
}

// registerEngSpecControl installs the fixture `if` and `for`.
func registerEngSpecControl(r *eng.Registry) {
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "if",
		Signatures: []eng.Signature{{
			Args:       []*eng.Type{eng.TAny, eng.TAny, eng.TAny},
			NoEvalArgs: map[int]bool{0: true, 1: true, 2: true},
			Impl:       eng.Go(fixIf3Handler),
			ReturnsFn:  fixIf3ReturnsFn, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "for",
		Signatures: []eng.Signature{
			{
				Args:       []*eng.Type{eng.TInteger, eng.TList},
				NoEvalArgs: map[int]bool{1: true},
				Impl:       eng.Go(fixForCountHandler),
				ReturnsFn:  fixForReturnsFn, BarrierPos: -1,
			},
			{
				Args:       []*eng.Type{eng.TList, eng.TList},
				NoEvalArgs: map[int]bool{1: true},
				Impl:       eng.Go(fixForRangeHandler),
				ReturnsFn:  fixForRangeReturnsFn, BarrierPos: -1,
			},
		},
	})
}

// fixDoHandler mirrors basic's DoListHandler: run the body with no
// inputs through the InvokeBody seam (so the VM can run it as a
// compiled closure) and surface a body error as an Error VALUE.
func fixDoHandler(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, r *eng.Registry) ([]eng.Value, error) {
	if !eng.IsConcrete(args[0]) { //covergate:allow the body is a dispatch-matched List argument and handlers never run on carriers; a bare List literal cannot bind the sig slot
		return nil, &eng.BoruError{Code: "do_error", Detail: "doq: argument must be a concrete list, got type literal"}
	}
	result, err := eng.InvokeBody(r, args[0], nil)
	if err != nil {
		return []eng.Value{eng.NewError(err)}, nil
	}
	return result, nil
}

// fixDoReturnsFn is the closure model: a computed body takes the
// bounded dynamic hatch, and a concrete body's residual is analysed
// under the caught-body bracket with do's keep-defs leak fidelity.
// Check dispatch resolves def'd words before the ReturnsFn runs (only
// builtin TYPE names pass through raw — resolveTypeNameArgs), so the
// body arrives as a value, never a Word. Raising bodies are outside
// the fixture's model — battery rows keep bodies infallible.
func fixDoReturnsFn(args []eng.Value, r *eng.Registry) []eng.Value {
	body := args[0]
	if !(eng.IsConcrete(body) && body.Parent.ConformsTo(eng.TList)) {
		return []eng.Value{eng.NewDynamicCarrier(eng.TAny)}
	}
	r.Check.CaughtBodyDepth++
	stk := eng.RunCarrierBodyKeepDefs(r, body)
	r.Check.CaughtBodyDepth--
	if len(stk) == 0 {
		if bl, err := eng.AsList(body); err == nil && !bl.IsNil() && bl.Len() > 0 {
			return []eng.Value{eng.NewCarrier(eng.TError)}
		}
		return nil
	}
	return stk
}

// registerEngSpecCallable installs `doq` — the battery's Callable
// body-runner, carrying basic's do CallableSpec so the kernel's
// closure dispatch/record/lower pipeline is driven standalone — and
// `dofbq`, the fallback-only variant (CompileFallbackBody without
// CompileDynBody, the each/fold shape): a body the closure path
// declines lowers to an interpreter ISLAND instead of a DynEnv call,
// driving the fallback-span record/lower/VM path.
func registerEngSpecCallable(r *eng.Registry) {
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "dofbq",
		Callable: &eng.CallableSpec{BodyPos: 0, BodyOut: eng.BodyOutResidual, Inputs: func(_ []eng.Value) []eng.Value {
			return []eng.Value{}
		}},
		Signatures: []eng.Signature{{
			Args:       []*eng.Type{eng.TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       eng.Go(fixDoHandler),
			ReturnsFn:  fixDoReturnsFn, BarrierPos: -1,
			CompileEffect: eng.CompileFallbackBody,
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "doq",
		Callable: &eng.CallableSpec{BodyPos: 0, BodyOut: eng.BodyOutResidual, Inputs: func(_ []eng.Value) []eng.Value {
			return []eng.Value{}
		}},
		Signatures: []eng.Signature{{
			Args:       []*eng.Type{eng.TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       eng.Go(fixDoHandler),
			ReturnsFn:  fixDoReturnsFn, BarrierPos: -1,
			CompileEffect: eng.CompileFallbackBody | eng.CompileDynBody,
		}},
	})
}

// fixParseRange decodes a for-range list mirroring basic's ParseRange:
// [end] / [start end] / [start end step], every bound a concrete
// Integer (the VM's OpForSetup taxonomy).
func fixParseRange(elems []eng.Value) (start, end, step int64, err error) {
	intAt := func(v eng.Value) (int64, error) {
		n, e := v.AsConcreteInteger()
		if e != nil {
			return 0, &eng.BoruError{Code: "for_error", Detail: "for: range: expected a concrete integer"}
		}
		return n, nil
	}
	switch len(elems) {
	case 1:
		if end, err = intAt(elems[0]); err != nil {
			return 0, 0, 0, err
		}
		return 0, end, 1, nil
	case 2:
		if start, err = intAt(elems[0]); err != nil {
			return 0, 0, 0, err
		}
		if end, err = intAt(elems[1]); err != nil {
			return 0, 0, 0, err
		}
		return start, end, 1, nil
	case 3:
		if start, err = intAt(elems[0]); err != nil {
			return 0, 0, 0, err
		}
		if end, err = intAt(elems[1]); err != nil {
			return 0, 0, 0, err
		}
		if step, err = intAt(elems[2]); err != nil {
			return 0, 0, 0, err
		}
		if step == 0 {
			return 0, 0, 0, &eng.BoruError{Code: "for_error", Detail: "for: step cannot be zero"}
		}
		return start, end, step, nil
	}
	return 0, 0, 0, &eng.BoruError{Code: "for_error", Detail: "for: range must have 1-3 elements"}
}

// fixLoopIterations mirrors basic's LoopIterations for the static
// zero-prune and region sizing.
func fixLoopIterations(start, end, step int64) int64 {
	if step > 0 {
		if start >= end {
			return 0
		}
		return (end - start + step - 1) / step
	}
	if step < 0 {
		if start <= end {
			return 0
		}
		return (start - end - step - 1) / -step
	}
	return -1 //covergate:allow fixParseRange rejects step==0, so both signed arms above are exhaustive
}

func fixForRangeHandler(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, r *eng.Registry) ([]eng.Value, error) {
	if !eng.IsConcrete(args[0]) { //covergate:allow the range is a dispatch-matched List argument and handlers never run on carriers; a bare List literal cannot bind the sig slot
		return nil, &eng.BoruError{Code: "for_error", Detail: "for: range must be a concrete list, got type literal"}
	}
	lst, _ := eng.AsList(args[0])
	start, end, step, err := fixParseRange(lst.Slice())
	if err != nil {
		return nil, err
	}
	if step < 0 && start <= end {
		return nil, nil
	}
	return fixRunForLoop(r, start, end, step, "i", args[1])
}

// fixForRangeReturnsFn is the range-form model: a fully-static range
// decomposes for RecordLoop; a range whose bounds are computed keeps
// const start/step and resolves the end operand (basic's
// computedRangeBounds shape) so the lowering still emits FOR_SETUP.
func fixForRangeReturnsFn(args []eng.Value, r *eng.Registry) []eng.Value {
	body := args[1]
	iter := eng.NewCarrier(eng.TInteger)
	es := r.Check.Recorder()

	var startV, endV, stepV eng.Value
	lowerable, staticBounds := false, false
	staticCount := int64(-1)
	if cv := args[0]; eng.IsConcrete(cv) && cv.Parent.ConformsTo(eng.TList) {
		if lst, err := eng.AsList(cv); err == nil && !lst.IsNil() {
			elems := lst.Slice()
			if st, en, sp, perr := fixParseRange(elems); perr == nil {
				startV, endV, stepV = eng.NewInteger(st), eng.NewInteger(en), eng.NewInteger(sp)
				lowerable, staticBounds = true, true
				staticCount = fixLoopIterations(st, en, sp)
				if staticCount == 0 {
					return []eng.Value{}
				}
			} else {
				switch len(elems) {
				case 1:
					startV, endV, stepV = eng.NewInteger(0), elems[0], eng.NewInteger(1)
					lowerable = true
				case 2:
					startV, endV, stepV = elems[0], elems[1], eng.NewInteger(1)
					lowerable = true
				case 3:
					startV, endV, stepV = elems[0], elems[1], elems[2]
					lowerable = true
				}
			}
		}
	}
	if lowerable {
		es.ArmLoopCapture()
	}
	provenTrips := staticBounds && staticCount >= 1
	stk := eng.AnalyseLoopBody(r, body, []string{"i"}, []eng.Value{iter}, provenTrips)
	out := eng.NewCarrier(eng.TList)
	if len(stk) > 0 {
		top := stk[len(stk)-1]
		if eng.IsDisjunct(top) {
			out = eng.NewCarrierTypedListValue(top)
		} else {
			out = eng.NewCarrierTypedList(top.Parent)
		}
	}
	if lowerable {
		regionN := 0
		if staticBounds && staticCount >= 1 && len(stk) > 0 {
			regionN = int(staticCount) * len(stk)
		}
		frag := es.TakeFragment()
		es.RecordLoop(startV, endV, stepV, frag, stk, iter.ID, out, regionN, args[0].Pos())
	}
	if len(stk) == 0 && (!es.Active() || !lowerable) {
		return []eng.Value{}
	}
	const spreadCap = 256
	if !es.Active() && staticBounds && staticCount >= 0 && len(stk) > 0 && staticCount*int64(len(stk)) <= spreadCap {
		spread := make([]eng.Value, 0, int(staticCount)*len(stk))
		for i := int64(0); i < staticCount; i++ {
			spread = append(spread, stk...)
		}
		return spread
	}
	return []eng.Value{out}
}

// fixBakeFnHandler applies its fn argument to 7 — the runtime side of
// the battery's stored-fn word.
func fixBakeFnHandler(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, r *eng.Registry) ([]eng.Value, error) {
	info, ok := args[0].Data.(eng.FnDefInfo)
	if !ok || len(info.Signatures) == 0 {
		return nil, &eng.BoruError{Code: "type_error", Detail: "bakefnq: argument must be a fn"}
	}
	return r.CallBoru(&info.Signatures[0], []eng.Value{eng.NewInteger(7)}, nil)
}

// registerEngSpecBaking installs `bakefnq` and `bakebodyq` — battery
// fixtures declaring the stored-fn / stored-body compile capabilities
// (the spawn / service / codec-handler shapes), so recordCallOperands'
// baking arms and compileStoredBody run standalone.
func registerEngSpecBaking(r *eng.Registry) {
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "bakefnq",
		Signatures: []eng.Signature{{
			Args:          []*eng.Type{eng.TFunction},
			Impl:          eng.Go(fixBakeFnHandler),
			Returns:       []*eng.Type{eng.TAny},
			BarrierPos:    -1,
			CompileEffect: eng.CompileStoresFn,
		}},
	})
	r.RegisterNativeFunc(eng.NativeFunc{
		Name: "bakebodyq",
		Signatures: []eng.Signature{{
			Args:       []*eng.Type{eng.TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       eng.Go(fixDoHandler),
			ReturnsFn:  fixDoReturnsFn, BarrierPos: -1,
			CompileEffect: eng.CompileStoresBody,
		}},
	})
}
