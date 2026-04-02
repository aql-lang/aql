package engine

import "fmt"

func registerTable(r *Registry) {
	tableHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		target := args[0]
		if !target.IsRecordType() {
			return nil, fmt.Errorf("table: argument must be a record type, got %s", target.String())
		}
		return []Value{NewTableType(target.AsRecordType())}, nil
	}

	r.Register("table", Signature{
		Args:    []Type{TAny},
		Handler: tableHandler,
	})
}
