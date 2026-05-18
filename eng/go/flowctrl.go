package eng

import "fmt"

// FlowCtrl is a non-error signalling channel for control-flow primitives
// (break, continue, ...). It travels separately from `error` so the two
// concerns stay distinct: errors mean execution FAILED, FlowCtrl values
// mean execution should be REDIRECTED.
//
// Transport: handlers set Registry.FlowCtrl rather than returning an
// error sentinel. The Run loop reads it after every step. Because
// sub-engines share a Registry, the signal naturally propagates across
// nested Run frames without abusing the error return.
//
// The `break` and `continue` words themselves live in the lang layer
// (lang/go/engine/native_control.go) — eng only owns the FlowCtrl type
// plus the Run-loop dispatch. To extend the channel with a new
// signal, add a constant here and a matching case in the Run loop's
// post-step handler.
type FlowCtrl uint8

const (
	FlowNone FlowCtrl = iota
	FlowBreak
	FlowContinue
)

func (f FlowCtrl) String() string {
	switch f {
	case FlowNone:
		return "none"
	case FlowBreak:
		return "break"
	case FlowContinue:
		return "continue"
	default:
		return fmt.Sprintf("FlowCtrl(%d)", uint8(f))
	}
}
