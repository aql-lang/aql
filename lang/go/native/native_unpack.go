package native

// unpackNatives covers the destructuring word `unpack`, which extracts
// selected entries from a Map (or Record) value and binds each to a
// bare word in the current scope — AQL's analogue of JavaScript object
// destructuring (`const {from, where, select} = query`).
//
// Surface (forward form): `unpack [names] map`.
//
//	def m {x:1}
//	unpack [x] m            # binds x → 1 in the current scope
//	x                       # → 1
//
// The motivating use case is improving the SQL DX of aql:query: after
// `"aql:query" import`, the words live under a dot namespace
// (query.from, query.where, …). Destructuring lifts the chosen ones to
// bare names:
//
//	"aql:query" import
//	unpack [from where select] query
//	from people  where [age gt 18]  select [name age]
//
// Because module export values are FnDefInfo Values that already carry
// their sub-registry, re-binding the extracted value under a bare name
// preserves module-scope dispatch — no copying or re-wrapping.
//
// v1 supports explicit names only: the first argument is a literal list
// of bare words/atoms/strings. Rename pairs and bind-all/spread forms
// are intentionally out of scope (the design leaves room to add them).
var unpackNatives = []NativeFunc{
	{
		Name: "unpack",
		Signatures: []NativeSig{{
			// sig[0] = names list (forward, kept un-evaluated so the
			// bare words survive as names); sig[1] = source map/record.
			Args:           []*Type{TList, TMap},
			NoEvalArgs:     map[int]bool{0: true},
			Handler:        unpackHandler,
			Returns:        []*Type{},
			RunInCheckMode: true,
			BarrierPos:     -1,
		}},
	},
}

// unpackHandler binds each name in args[0] to the matching entry of the
// map/record args[1]. Missing keys are an error (strict, like getr),
// except in check mode where a missing key binds an Any carrier so
// downstream references still resolve during static analysis.
func unpackHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	names, err := RequireConcreteList(args[0], "unpack")
	if err != nil {
		return nil, err
	}

	// Resolve a key-lookup function over the source: a concrete map or a
	// record's field map. In check mode the source may be a non-concrete
	// carrier (e.g. an imported export whose value isn't materialised);
	// fall back to binding Any carriers so analysis can continue.
	var get func(string) (Value, bool)
	switch {
	case IsConcrete(args[1]):
		if m, mErr := AsMap(args[1]); mErr == nil && m != nil {
			get = m.Get
		} else if IsRecordType(args[1]) {
			rec, rErr := AsRecordType(args[1])
			if rErr != nil {
				return nil, r.AqlError("unpack_error", "unpack: source record is malformed", "unpack")
			}
			get = rec.Fields.Get
		} else {
			return nil, r.AqlError("unpack_error", "unpack: source must be a map or record", "unpack")
		}
	case r.Check.IsActive():
		get = func(string) (Value, bool) { return Value{}, false }
	default:
		return nil, r.AqlError("unpack_error", "unpack: source is not a concrete map", "unpack")
	}

	for _, el := range names.Slice() {
		name := defName(el)
		if name == "" {
			return nil, r.AqlError("unpack_error", "unpack: names must be words, atoms, or strings", "unpack")
		}
		if IsCapitalisedName(name) {
			return nil, r.AqlError("unpack_error", "unpack: cannot bind capitalised (type) name "+name+" — unpack binds values only", "unpack")
		}
		if err := ValidateWordName(name); err != nil {
			return nil, err
		}
		if r.Defs.IsType(name) {
			return nil, r.AqlError("unpack_error", "unpack: name clash — "+name+" is already a type", "unpack")
		}

		val, ok := get(name)
		if !ok {
			if r.Check.IsActive() {
				// Lenient: bind a generic value so later references to
				// the name don't cascade into undefined-word errors.
				val = NewCarrier(TAny)
			} else {
				return nil, r.AqlError("unpack_error", "unpack: key "+name+" not found in source", "unpack")
			}
		}
		if _, err := installAndRecordDef(r, name, val, el.Pos); err != nil {
			return nil, err
		}
	}
	return nil, nil
}
