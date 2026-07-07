package eng

import "testing"

// TestPayloadMarkersInvoke calls every payloadMarker (and the
// HostTypeBody marker) directly. The methods exist purely to satisfy
// the sealed Payload interface (see CLAUDE.md "Sealed Payload") and are
// never dispatched at runtime, so this is the only way to execute them.
func TestPayloadMarkersInvoke(t *testing.T) {
	payloads := []Payload{
		ReachInfo{},
		IntPayload{},
		FloatPayload{},
		BigIntPayload{},
		DecimalPayload{},
		StrPayload{},
		BoolPayload{},
		AtomPayload{},
		PathonPayload{},
		MicronPayload{},
		MicronTypeInfo{},
		ListPayload{},
		MapPayload{},
		ParenExprPayload{},
		InterpStringPayload{},
		TimePayload{},
		DurationPayload{},
		TimezonePayload{},
		MaterializerPayload{},
		NonePayload{},
		ExtensionPayload{},
		WordInfo{},
		ForwardInfo{},
		MarkInfo{},
		MoveInfo{},
		SpliceInfo{},
		ReturnCheckInfo{},
		DefCleanupInfo{},
		GuardFactInfo{},
		FrameOpenInfo{},
		ModuleDesc{},
		FnDefInfo{},
		FnUndefInfo{},
		DisjunctInfo{},
		NegationInfo{},
		ChildTypeInfo{},
		CodeEffectInfo{},
		RecordTypeInfo{},
		OptionsTypeInfo{},
		TableTypeInfo{},
		TableData{},
		ClassTypeInfo{},
		ClassInstanceInfo{},
		&SurfaceInfo{},
		&GenSpecInfo{},
		GenParam{},
		&TypeSchemaInfo{},
		GenInstRef{},
		&FlexListData{},
		XmlElementPayload{},
		XmlInterpPayload{},
		&FlexXmlData{},
		&StoreInstanceInfo{},
		&StoreShapeInfo{},
		ResourceTypeInfo{},
		ResourceInstanceInfo{},
		&TimeoutInfo{},
		&IntervalInfo{},
		ErrorInfo{},
		CalDurationData{},
		DepScalarInfo{},
		ClosurePayload{},
		PathonInfo{},
		noneSentinel{},
	}
	for _, p := range payloads {
		p.payloadMarker()
	}
	HostTypeBody{}.hostTypeBody()
}
