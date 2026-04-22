package engine

import "fmt"

func RegisterTable(r *Registry) {
	tableHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		target := args[0]
		if !target.IsRecordType() {
			return nil, fmt.Errorf("table: argument must be a record type, got %s", target.String())
		}
		_as0, _ := target.AsRecordType()
		return []Value{NewTableType(_as0)}, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "table",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args:           []Type{TAny},
			Handler:        tableHandler,
			Returns:        []Type{TTable},
			RunInCheckMode: true,
		}},
	})
}
