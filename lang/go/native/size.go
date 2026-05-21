package native

// The "size" word is registered via the consolidated Natives slice in
// natives.go. It reports the natural size of any value through the
// kernel's Sizer behaviour — see eng.SizeOf for the per-type rules.
func sizeHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewInteger(int64(SizeOf(args[0])))}, nil
}
