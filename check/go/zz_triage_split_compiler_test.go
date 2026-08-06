package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

func TestValueTreeHasCarriers(t *testing.T) {
	if valueTreeHasCarriers(core.NewInteger(1)) {
		t.Error("plain int has carriers")
	}
	if !valueTreeHasCarriers(core.NewCarrier(core.TInteger)) {
		t.Error("carrier missed")
	}
	lst := core.NewList([]core.Value{core.NewInteger(1), core.NewCarrier(core.TString)})
	if !valueTreeHasCarriers(lst) {
		t.Error("nested list carrier missed")
	}
	om := core.NewOrderedMap()
	om.Set("a", core.NewList([]core.Value{core.NewDynamicCarrier(core.TAny)}))
	if !valueTreeHasCarriers(core.NewMap(om)) {
		t.Error("nested map carrier missed")
	}
	om2 := core.NewOrderedMap()
	om2.Set("a", core.NewInteger(1))
	if valueTreeHasCarriers(core.NewMap(om2)) {
		t.Error("concrete map has carriers")
	}
}
