package basic

// Register installs the basic layer on the given registry: the
// forth-style stack words, the definition family (def / undef / var /
// fn / afn / fnsig / args), the type-generics words (gen / extends /
// default / of), `const`, the predefined Resource / Entity type
// definitions, and the control-flow words (do / if / case / for /
// break / continue / error). The relative order mirrors the order
// lang's Register applies the same groups.
//
// lang does NOT call this: its Register interleaves these groups with
// the rest of the word library at the historical registration points
// (overload appends are order-sensitive there). Register exists for
// eng-only embedders that want the fundamental words without the full
// language layer — the calc/go pattern, one layer up.
func Register(r *Registry) {
	for _, n := range StackNatives {
		r.RegisterNativeFunc(n)
	}
	for _, n := range DefinitionNatives {
		r.RegisterNativeFunc(n)
	}
	for _, n := range GenNatives {
		r.RegisterNativeFunc(n)
	}
	r.RegisterNativeFunc(ConstNative)
	InstallResourceTypes(r)
	for _, n := range ControlNatives {
		r.RegisterNativeFunc(n)
	}
	// After every constructor slice above: the def keyword forms are
	// synthesized from the constructors' live signature tables.
	RegisterDefKeywordForms(r)
}
