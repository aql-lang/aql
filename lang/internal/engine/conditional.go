package engine

// spliceArg returns tokens for a branch value. If the value is a list,
// its elements are returned wrapped in parens so the main engine evaluates
// them as a sub-expression. Scalars are returned as-is.
func spliceArg(v Value) []Value {
	if v.VType.Equal(TList) && v.Data != nil && !v.IsTypedList() && !v.IsTableType() {
		elems := v.AsList()
		result := make([]Value, 0, elems.Len()+2)
		result = append(result, NewOpenParen())
		result = append(result, elems.Slice()...)
		result = append(result, NewWord(")"))
		return result
	}
	return []Value{v}
}
