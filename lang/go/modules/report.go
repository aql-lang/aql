package modules

import (
	"fmt"
	"strings"

	"github.com/aql-lang/aql/lang/go/native"
)

// BuildReportModule creates the "aql:report" native module. It exposes
// pretty-printers for the kernel value types — primarily Records and
// Tables, but useful for any Value. Each word returns a String so
// callers can compose with `print`, error messages, or further
// formatting. No word prints to stdout itself; the caller controls IO.
//
//	"aql:report" import
//	some-record report.record print
//	some-table  report.table  print
//	some-value  report.value  print
func BuildReportModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}

	for _, n := range reportNatives() {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()
	exports.Set("value", makeReportFnDef("report-value", subReg))
	exports.Set("record", makeReportFnDef("report-record", subReg))
	exports.Set("table", makeReportFnDef("report-table", subReg))
	exports.Set("list", makeReportFnDef("report-list", subReg))

	modID := parent.Modules.NextID()
	return native.ModuleDesc{
		ID:      modID,
		Exports: map[string]*native.OrderedMap{"report": exports},
	}, nil
}

func makeReportFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{
		Name: wordName,
		Sigs: []native.FnSig{{
			Params:     []native.FnParam{{Type: native.TAny}},
			Returns:    []*native.Type{native.TString},
			Body:       []native.Value{native.NewWord(wordName)},
			BarrierPos: -1,
		}},
		Registry: subReg,
	})
}

func reportNatives() []native.NativeFunc {
	return []native.NativeFunc{
		{
			Name: "report-value",
			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TAny},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewString(native.FormatForPrint(args[0]))}, nil
				},
				Returns: []*native.Type{native.TString}, BarrierPos: -1,
			}},
		},
		{
			Name: "report-record",
			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TAny},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewString(formatRecord(args[0]))}, nil
				},
				Returns: []*native.Type{native.TString}, BarrierPos: -1,
			}},
		},
		{
			Name: "report-table",
			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TAny},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewString(formatTableValue(args[0]))}, nil
				},
				Returns: []*native.Type{native.TString}, BarrierPos: -1,
			}},
		},
		{
			Name: "report-list",
			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TAny},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewString(formatListValue(args[0]))}, nil
				},
				Returns: []*native.Type{native.TString}, BarrierPos: -1,
			}},
		},
	}
}

// formatRecord renders a Map as a vertical key:value block aligned on
// the colon. Falls back to FormatForPrint for non-Map values so the
// word is safe to call on heterogeneous inputs.
func formatRecord(v native.Value) string {
	if !native.IsConcrete(v) {
		return native.FormatForPrint(v)
	}
	m, err := native.RequireConcreteMap(v, "report.record")
	if err != nil {
		return native.FormatForPrint(v)
	}
	keys := m.Keys()
	if len(keys) == 0 {
		return "(empty record)"
	}
	maxKey := 0
	for _, k := range keys {
		if len(k) > maxKey {
			maxKey = len(k)
		}
	}
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('\n')
		}
		val, _ := m.Get(k)
		b.WriteString(native.PadRight(k, maxKey))
		b.WriteString(" : ")
		b.WriteString(native.FormatForPrint(val))
	}
	return b.String()
}

// formatTableValue renders a TableData with aligned columns and a
// header row. When v is a plain list of maps, the columns are derived
// from the union of keys (in first-seen order) and an ad-hoc table is
// rendered.
func formatTableValue(v native.Value) string {
	if !native.IsConcrete(v) {
		return native.FormatForPrint(v)
	}
	if native.IsTableType(v) {
		return native.FormatForPrint(v)
	}
	if !v.Parent.Equal(native.TList) {
		return native.FormatForPrint(v)
	}
	rows, err := native.AsList(v)
	if err != nil {
		return native.FormatForPrint(v)
	}
	if rows.Len() == 0 {
		return "(empty table)"
	}
	cols := []string{}
	seen := map[string]bool{}
	for _, row := range rows.Slice() {
		if !row.Parent.Equal(native.TMap) || !native.IsConcrete(row) {
			return native.FormatForPrint(v)
		}
		rm, _ := native.AsMap(row)
		if rm == nil {
			return native.FormatForPrint(v)
		}
		for _, k := range rm.Keys() {
			if !seen[k] {
				seen[k] = true
				cols = append(cols, k)
			}
		}
	}
	cells := make([][]string, rows.Len())
	for i, row := range rows.Slice() {
		cells[i] = make([]string, len(cols))
		rm, _ := native.AsMap(row)
		for j, c := range cols {
			if cv, ok := rm.Get(c); ok {
				cells[i][j] = native.FormatForPrint(cv)
			}
		}
	}
	widths := make([]int, len(cols))
	for j, c := range cols {
		widths[j] = len(c)
	}
	for _, r := range cells {
		for j, cell := range r {
			if len(cell) > widths[j] {
				widths[j] = len(cell)
			}
		}
	}
	var b strings.Builder
	for j, c := range cols {
		if j > 0 {
			b.WriteString(" | ")
		}
		b.WriteString(native.PadRight(c, widths[j]))
	}
	b.WriteByte('\n')
	for j := range cols {
		if j > 0 {
			b.WriteString("-+-")
		}
		b.WriteString(strings.Repeat("-", widths[j]))
	}
	b.WriteByte('\n')
	for i, r := range cells {
		if i > 0 {
			b.WriteByte('\n')
		}
		for j, cell := range r {
			if j > 0 {
				b.WriteString(" | ")
			}
			b.WriteString(native.PadRight(cell, widths[j]))
		}
	}
	return b.String()
}

// formatListValue renders a list one element per line, indexed.
// Falls back to FormatForPrint when v is not a concrete list.
func formatListValue(v native.Value) string {
	if !native.IsConcrete(v) {
		return native.FormatForPrint(v)
	}
	if !v.Parent.Equal(native.TList) {
		return native.FormatForPrint(v)
	}
	rows, err := native.AsList(v)
	if err != nil {
		return native.FormatForPrint(v)
	}
	if rows.Len() == 0 {
		return "(empty list)"
	}
	idxWidth := len(fmt.Sprintf("%d", rows.Len()-1))
	var b strings.Builder
	for i, e := range rows.Slice() {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(native.PadRight(fmt.Sprintf("[%d]", i), idxWidth+2))
		b.WriteByte(' ')
		b.WriteString(native.FormatForPrint(e))
	}
	return b.String()
}
