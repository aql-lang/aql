package eng

import "strings"

// listFormatBehavior renders a List value, dispatching on the
// internal payload sub-shape (TableTypeInfo, TableData,
// MaterializerPayload, ChildTypeInfo, plain ListPayload). Moved
// from Value.String at Step 10 — the kernel's render switch no
// longer carries TList sub-payload logic; the Behavior owns it.
type listFormatBehavior struct{}

func (listFormatBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (listFormatBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (listFormatBehavior) Format(v Value) string {
	if tt, ok := v.Data.(TableTypeInfo); ok {
		parts := make([]string, 0, tt.Record.Fields.Len())
		for _, k := range tt.Record.Fields.Keys() {
			val, _ := tt.Record.Fields.Get(k)
			parts = append(parts, k+":"+val.String())
		}
		return "table{" + strings.Join(parts, ",") + "}"
	}
	if td, ok := v.Data.(TableData); ok {
		parts := make([]string, 0, td.Record.Fields.Len())
		for _, k := range td.Record.Fields.Keys() {
			val, _ := td.Record.Fields.Get(k)
			parts = append(parts, k+":"+val.String())
		}
		rowParts := make([]string, len(td.Rows))
		for i, row := range td.Rows {
			rowParts[i] = row.String()
		}
		return "table{" + strings.Join(parts, ",") + "}[" + strings.Join(rowParts, ",") + "]"
	}
	if mp, ok := v.Data.(MaterializerPayload); ok {
		td, err := mp.M.Materialize()
		if err != nil {
			return "query(error:" + err.Error() + ")"
		}
		return formatTableDataAsList(td)
	}
	if mz, ok := v.Data.(Materializer); ok {
		td, err := mz.Materialize()
		if err != nil {
			return "query(error:" + err.Error() + ")"
		}
		return formatTableDataAsList(td)
	}
	if ct, ok := v.Data.(ChildTypeInfo); ok {
		return "[:" + ct.Child.String() + "]"
	}
	_lst, _ := AsList(v)
	elems := _lst.Slice()
	parts := make([]string, len(elems))
	for i, e := range elems {
		parts[i] = e.String()
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// formatTableDataAsList renders TableData by re-wrapping it in a
// transient List value and calling String — preserving the
// historical render path for materialized query results.
func formatTableDataAsList(td TableData) string {
	v := NewValueRaw(TList, td)
	return v.String()
}

// mapFormatBehavior renders a Map value, dispatching on the
// internal payload sub-shape (ChildTypeInfo, RecordTypeInfo,
// OptionsTypeInfo, plain MapPayload). Moved from Value.String at
// Step 10.
type mapFormatBehavior struct{}

func (mapFormatBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (mapFormatBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (mapFormatBehavior) Format(v Value) string {
	if ct, ok := v.Data.(ChildTypeInfo); ok {
		return "{:" + ct.Child.String() + "}"
	}
	if rt, ok := v.Data.(RecordTypeInfo); ok {
		parts := make([]string, 0, rt.Fields.Len())
		for _, k := range rt.Fields.Keys() {
			val, _ := rt.Fields.Get(k)
			parts = append(parts, k+":"+val.String())
		}
		return "record{" + strings.Join(parts, ",") + "}"
	}
	if ot, ok := v.Data.(OptionsTypeInfo); ok {
		parts := make([]string, 0, ot.Fields.Len())
		for _, k := range ot.Fields.Keys() {
			val, _ := ot.Fields.Get(k)
			parts = append(parts, k+":"+val.String())
		}
		return "options{" + strings.Join(parts, ",") + "}"
	}
	m, _ := AsMap(v)
	if m == nil {
		return "{}"
	}
	parts := make([]string, 0, m.Len())
	for _, k := range m.Keys() {
		val, _ := m.Get(k)
		parts = append(parts, k+":"+val.String())
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func init() {
	TList.Behavior = listFormatBehavior{}
	TMap.Behavior = mapFormatBehavior{}
}
