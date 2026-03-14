package engine

func registerTypeof(r *Registry) {
	typeofHandler := func(args []Value) ([]Value, error) {
		v := args[0]
		name := v.VType.Parts[0]
		return []Value{NewAtom(name)}, nil
	}

	r.Register("typeof",
		Signature{Args: []Type{TAny}, Handler: typeofHandler},
	)
}
