package eng

// Seam-7 (cluster A4): in-package unit tests for previously-unreached
// blocks in print.go and indexcheck.go. Renderers are driven with crafted
// Values; the check-mode index helpers are called directly with
// DepScalar / concrete / malformed operands. Per design/TEST-SEAMS.10.md.

import (
	"errors"
	"strings"
	"testing"
)

// s7Mz is a test Materializer that can be stored directly as a payload
// (payloadMarker) or wrapped in a MaterializerPayload.
type s7Mz struct{ err error }

func (m s7Mz) Materialize() (TableData, error) {
	return TableData{Record: RecordTypeInfo{Fields: NewOrderedMap()}}, m.err
}
func (s7Mz) SourceRecord() RecordTypeInfo { return RecordTypeInfo{Fields: NewOrderedMap()} }
func (s7Mz) payloadMarker()               {}

// --- print.go -------------------------------------------------------------

func TestS7FormatForPrintMaterializer(t *testing.T) {
	boom := errors.New("boom")
	// MaterializerPayload, erroring.
	v := NewValueRaw(TList, MaterializerPayload{M: s7Mz{err: boom}})
	if got := FormatForPrint(v); !strings.Contains(got, "error") {
		t.Errorf("materializer-payload error render = %q", got)
	}
	// MaterializerPayload, succeeding -> formatTable.
	ok := NewValueRaw(TList, MaterializerPayload{M: s7Mz{}})
	if got := FormatForPrint(ok); got == "" {
		t.Error("materializer-payload ok render should not be empty")
	}
	// Direct Materializer payload, erroring.
	dv := NewValueRaw(TList, s7Mz{err: boom})
	if got := FormatForPrint(dv); !strings.Contains(got, "error") {
		t.Errorf("materializer error render = %q", got)
	}
	// Direct Materializer payload, succeeding.
	dok := NewValueRaw(TList, s7Mz{})
	if got := FormatForPrint(dok); got == "" {
		t.Error("materializer ok render should not be empty")
	}
}

func TestS7FormatForPrintOptions(t *testing.T) {
	opt := NewValueRaw(TMap, OptionsTypeInfo{Fields: NewOrderedMap()})
	if got := FormatForPrint(opt); got == "" {
		t.Error("options print should not be empty")
	}
}

func TestS7FormatMapJSONError(t *testing.T) {
	// AsMutableMap fails on a record payload -> "{}".
	rec := NewValueRaw(TMap, RecordTypeInfo{Fields: NewOrderedMap()})
	if got := formatMapJSON(rec); got != "{}" {
		t.Errorf("formatMapJSON on a non-mutable map = %q, want {}", got)
	}
}

func TestS7FormatValueJSONArms(t *testing.T) {
	if got := FormatValueJSON(NewTypeLiteral(TInteger)); got != `"Integer"` {
		t.Errorf("type-literal JSON = %q", got)
	}
	if got := FormatValueJSON(NewDepScalar(DepGT, NewInteger(10))); !strings.Contains(got, "gt") {
		t.Errorf("depscalar JSON = %q", got)
	}
	// A None-parent value that is not the none sentinel -> "null" case arm.
	if got := FormatValueJSON(Value{Parent: TNone, Data: IntPayload{N: 1}}); got != "null" {
		t.Errorf("None-parent JSON = %q, want null", got)
	}
	// A float takes the default arm.
	if got := FormatValueJSON(NewFloat(1.5)); got == "" {
		t.Error("float JSON default should not be empty")
	}
}

// --- indexcheck.go --------------------------------------------------------

func TestS7StaticListLenNilList(t *testing.T) {
	// A concrete but nil list reports no static length.
	if _, ok := StaticListLen(NewList(nil)); ok {
		t.Error("a nil list must not report a static length")
	}
}

func TestS7MinMaxIndexErrorArms(t *testing.T) {
	// DepScalar whose bound value is a non-integer -> no min/max.
	strLo := NewDepScalar(DepGT, NewString("z"))
	if _, ok := minIndex(strLo); ok {
		t.Error("string-bounded lower DepScalar has no integer minimum")
	}
	strHi := NewDepScalar(DepLT, NewString("z"))
	if _, ok := maxIndex(strHi); ok {
		t.Error("string-bounded upper DepScalar has no integer maximum")
	}
	// Concrete Integer-tagged value with a non-int payload -> no min/max.
	badInt := Value{Parent: TInteger, Data: StrPayload{S: "x"}}
	if _, ok := minIndex(badInt); ok {
		t.Error("unreadable integer index has no minimum")
	}
	if _, ok := maxIndex(badInt); ok {
		t.Error("unreadable integer index has no maximum")
	}
}

func TestS7CheckAtIndicesGuards(t *testing.T) {
	r := newTestRegistry(t)
	// Unknown data length (non-list) -> no-op, no diagnostic.
	CheckAtIndices(r, NewList([]Value{NewInteger(0)}), NewInteger(1), "at")
	// Known data length, but the indices list is nil -> early return.
	data := NewList([]Value{NewInteger(1)})
	CheckAtIndices(r, NewList(nil), data, "at")
	if n := len(r.Check.Diagnostics); n != 0 {
		t.Errorf("guarded CheckAtIndices should emit nothing, got %d diagnostics", n)
	}
}
