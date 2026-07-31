package modules

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// s7bTable builds a concrete TableData-backed List value (IsTableType true,
// AsList unreadable) for the report formatters' Table arms.
func s7bTable() native.Value {
	rec := native.NewOrderedMap()
	rec.Set("a", native.NewTypeLiteral(native.TInteger))
	row := native.NewOrderedMap()
	row.Set("a", native.NewInteger(1))
	return native.NewValueRaw(native.TList, native.TableData{
		Record: native.RecordTypeInfo{Fields: rec},
		Rows:   []native.Value{native.NewMap(row)},
	})
}

// TestS7B_ReportNonConcreteFallback drives the !IsConcrete fallback arm of
// all three container formatters: a bare type literal renders via
// FormatForPrint, never by walking a nil payload.
func TestS7B_ReportNonConcreteFallback(t *testing.T) {
	litM := native.NewTypeLiteral(native.TMap)
	if got, want := formatRecord(litM), native.FormatForPrint(litM); got != want {
		t.Errorf("formatRecord(type literal) = %q, want %q", got, want)
	}
	litL := native.NewTypeLiteral(native.TList)
	if got, want := formatTableValue(litL), native.FormatForPrint(litL); got != want {
		t.Errorf("formatTableValue(type literal) = %q, want %q", got, want)
	}
	if got, want := formatListValue(litL), native.FormatForPrint(litL); got != want {
		t.Errorf("formatListValue(type literal) = %q, want %q", got, want)
	}
}

// TestS7B_ReportTableTypeArm drives formatTableValue's IsTableType arm:
// a real Table (TableData payload) renders via FormatForPrint rather than
// through the ad-hoc list-of-maps table builder.
func TestS7B_ReportTableTypeArm(t *testing.T) {
	tbl := s7bTable()
	if got, want := formatTableValue(tbl), native.FormatForPrint(tbl); got != want {
		t.Errorf("formatTableValue(Table) = %q, want %q", got, want)
	}
}

// TestS7B_ReportTableNonMapRows drives the "row is not a concrete Map" arm:
// a list whose elements are scalars renders through FormatForPrint.
func TestS7B_ReportTableNonMapRows(t *testing.T) {
	r := reportRegistry(t)
	got := runReportBORU(t, r, `[1 2 3] Report.table`)
	s, _ := native.AsString(got[0])
	want := native.FormatForPrint(native.NewList([]native.Value{
		native.NewInteger(1), native.NewInteger(2), native.NewInteger(3),
	}))
	if s != want {
		t.Errorf("Report.table on non-map rows = %q, want %q", s, want)
	}
}

// TestS7B_ReportTableWideCell drives the column-width update arm: a cell
// wider than its header widens the column.
func TestS7B_ReportTableWideCell(t *testing.T) {
	r := reportRegistry(t)
	got := runReportBORU(t, r, `[{a:12345}] Report.table`)
	s, _ := native.AsString(got[0])
	if !strings.Contains(s, "12345") {
		t.Errorf("wide cell not rendered in full:\n%s", s)
	}
}
