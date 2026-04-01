package engine

import "fmt"

func registerRecord(r *Registry) {
	recordHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		list := args[0]
		if !list.VType.Equal(TList) {
			return nil, fmt.Errorf("record: argument must be a list")
		}
		if list.Data == nil {
			return nil, fmt.Errorf("record: argument must be a concrete list, got type literal")
		}
		elems := list.AsList()
		if elems.Len() == 0 {
			return nil, fmt.Errorf("record: list must have at least one field")
		}
		fields := NewOrderedMap()
		for _, elem := range elems.Slice() {
			if !elem.VType.Equal(TMap) {
				return nil, fmt.Errorf("record: each element must be a pair (map), got %s", elem.String())
			}
			m, ok := elem.Data.(*OrderedMap)
			if !ok {
				return nil, fmt.Errorf("record: each element must be a concrete pair, got %s", elem.String())
			}
			for _, key := range m.Keys() {
				val, _ := m.Get(key)
				val = resolveFieldType(r, val)
				fields.Set(key, val)
			}
		}
		return []Value{NewRecordType(fields)}, nil
	}

	r.Register("record", Signature{
		Args:    []Type{TList},
		Handler: recordHandler,
	})
}
