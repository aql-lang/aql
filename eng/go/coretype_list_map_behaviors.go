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
		return formatFieldBag("table", tt.Record.Fields)
	}
	if td, ok := v.Data.(TableData); ok {
		rowParts := make([]string, len(td.Rows))
		for i, row := range td.Rows {
			rowParts[i] = row.String()
		}
		return formatFieldBag("table", td.Record.Fields) + "[" + strings.Join(rowParts, ",") + "]"
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

// formatFieldBag renders an OrderedMap of field name → value as
// `name{k:v,…}` — the shared render shape of record / options /
// table-schema type values.
func formatFieldBag(name string, fields *OrderedMap) string {
	parts := make([]string, 0, fields.Len())
	for _, k := range fields.Keys() {
		val, _ := fields.Get(k)
		parts = append(parts, k+":"+val.String())
	}
	return name + "{" + strings.Join(parts, ",") + "}"
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
		return formatFieldBag("record", rt.Fields)
	}
	if ot, ok := v.Data.(OptionsTypeInfo); ok {
		return formatFieldBag("options", ot.Fields)
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
