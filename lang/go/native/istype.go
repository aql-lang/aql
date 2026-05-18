package native

// The "istype" word is registered via the consolidated Natives slice in
// natives.go. It reports whether the input is a type literal/Options/Node
// containing a type leaf.
func istypeHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewBoolean(IsTypeValue(args[0]))}, nil
}
