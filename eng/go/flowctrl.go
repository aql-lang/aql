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
// Extension: add a new constant here and a matching case in the Run
// loop's post-step handler (and, if needed, a corresponding stack-tape
// resolver alongside handleLoopBreak / handleLoopContinue).
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

// RegisterCoreFlowCtrl installs the break and continue words. Their
// handlers raise a flow-control signal on the registry instead of
// returning an error.
func RegisterCoreFlowCtrl(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "break",
		Signatures: []NativeSig{{
			Handler: breakHandler,
			Returns: []*Type{},
		}},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name: "continue",
		Signatures: []NativeSig{{
			Handler: continueHandler,
			Returns: []*Type{},
		}},
	})
}

func breakHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	r.FlowCtrl = FlowBreak
	return nil, nil
}

func continueHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	r.FlowCtrl = FlowContinue
	return nil, nil
}
