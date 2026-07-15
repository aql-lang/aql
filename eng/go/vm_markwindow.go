package eng

// callDynMixedFromMark executes OpCallDynMixedFromMark — the variadic-region
// verbatim window (plan Phase 5, L-DO part 2b): island stack[mark:] through
// the SAME re-step machinery as CALL_DYNAMIC_MIXED, with the window width
// decided by the RUNTIME region (a fallible do-catch residual: 1 caught Error
// vs N values) plus the fixed values the preceding events pushed above it.
// The topmost mark is consumed; an empty window is a no-op (the mark still
// pops). Extracted from the run loop's dispatch for the cognitive-complexity
// cap (the dispatchRematch precedent).
// vmMarkOp routes the mark-family opcodes: the three bookkeeping ops go to
// vmMark; the mark-window island needs the vmContext (its island runner), so
// it dispatches here — keeping the run loop's mark case a single line under
// the cognitive-complexity cap.
func (vc *vmContext) vmMarkOp(reg *Registry, op Opcode, marks []int, stack []Value, curDebug []SrcPos, pc int) ([]int, []Value, error) {
	if op == OpCallDynMixedFromMark {
		return vc.callDynMixedFromMark(reg, marks, stack, curDebug, pc)
	}
	return vmMark(op, marks, stack, curDebug, pc)
}

func (vc *vmContext) callDynMixedFromMark(reg *Registry, marks []int, stack []Value, curDebug []SrcPos, pc int) ([]int, []Value, error) {
	if len(marks) == 0 {
		return nil, nil, vmErrAt(curDebug, pc, "CALL_DYN_MIXED_FROM_MARK with no open mark")
	}
	base := marks[len(marks)-1]
	marks = marks[:len(marks)-1]
	if base > len(stack) {
		return nil, nil, vmErrAt(curDebug, pc, "CALL_DYN_MIXED_FROM_MARK mark above stack top")
	}
	if w := len(stack) - base; w > 0 {
		var err error
		if stack, err = vc.callDynamicMixed(reg, w, stack, curDebug, pc); err != nil {
			return nil, nil, err
		}
	}
	return marks, stack, nil
}
