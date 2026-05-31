package native

import "fmt"

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
//	unpack [select from where] query
//	select [name age] from people where [age gt 18]
//
// Because module export values are FnDefInfo Values that already carry
// their sub-registry, re-binding the extracted value under a bare name
// preserves module-scope dispatch — no copying or re-wrapping.
//
// Three selector forms (sig[0]), all over the same source map/record
// (sig[1]):
//
//   - `unpack [names] map`     — explicit names: bind each listed key.
//   - `unpack all map`         — bind every key of the source.
//   - `unpack {renames} map`   — rename: each entry `srcKey: localName`
//     binds source key `srcKey` to the bare
//     word `localName`.
//
// Examples:
//
//	def m {a:1 b:2}
//	unpack [a] m          # binds a → 1
//	unpack all m          # binds a → 1 and b → 2
//	unpack {a: x b: y} m  # binds x → 1 and y → 2
var unpackNatives = []NativeFunc{
	{
		Name: "unpack",
		Signatures: []NativeSig{
			// `unpack [names] map`: explicit names list. NoEvalArgs[0]
			// keeps the bare words un-evaluated so they survive as names.
			{
				Args:           []*Type{TList, TMap},
				NoEvalArgs:     map[int]bool{0: true},
				Handler:        unpackHandler,
				Returns:        []*Type{},
				RunInCheckMode: true,
				BarrierPos:     -1,
			},
			// `unpack {renames} map`: the first map's entries drive the
			// bindings (srcKey → localName). NoEvalMapArgs[0] keeps the
			// target names un-evaluated so they survive as bare words.
			{
				Args:           []*Type{TMap, TMap},
				NoEvalMapArgs:  map[int]bool{0: true},
				Handler:        unpackRenameHandler,
				Returns:        []*Type{},
				RunInCheckMode: true,
				BarrierPos:     -1,
			},
			// `unpack all map`: the `all` keyword (captured as an atom via
			// /q even though it is a registered word) binds every key.
			{
				Args:           []*Type{TAtom, TMap},
				QuoteArgs:      map[int]bool{0: true},
				Handler:        unpackAllHandler,
				Returns:        []*Type{},
				RunInCheckMode: true,
				BarrierPos:     -1,
			},
		},
	},
}

// unpackSource resolves the source (sig[1]) to a keyed lookup plus its
// ordered key list. Accepts a concrete map or a record's field map. In
// check mode a non-concrete source (e.g. an imported export not yet
// materialised) yields an empty getter so analysis can continue with
// Any carriers rather than erroring.
func unpackSource(src Value, r *Registry) (get func(string) (Value, bool), keys []string, err error) {
	switch {
	case IsConcrete(src):
		if m, mErr := AsMap(src); mErr == nil && m != nil {
			return m.Get, m.Keys(), nil
		}
		if IsRecordType(src) {
			rec, rErr := AsRecordType(src)
			if rErr != nil {
				return nil, nil, r.AqlError("unpack_error", "unpack: source record is malformed", "unpack")
			}
			return rec.Fields.Get, rec.Fields.Keys(), nil
		}
		return nil, nil, r.AqlError("unpack_error", "unpack: source must be a map or record", "unpack")
	case r.Check.IsActive():
		return func(string) (Value, bool) { return Value{}, false }, nil, nil
	default:
		return nil, nil, r.AqlError("unpack_error", "unpack: source is not a concrete map", "unpack")
	}
}

// bindUnpackEntry binds localName to the source entry under srcKey.
// Validates the target name (rejects capitalised/type-clashing names),
// and is strict on missing keys except in check mode (where it binds an
// Any carrier so later references still resolve). pos locates the name
// token for unused-def analysis.
func bindUnpackEntry(r *Registry, localName, srcKey string, get func(string) (Value, bool), pos SrcPos) error {
	if localName == "" {
		return r.AqlError("unpack_error", "unpack: names must be words, atoms, or strings", "unpack")
	}
	if IsCapitalisedName(localName) {
		return r.AqlError("unpack_error", "unpack: cannot bind capitalised (type) name "+localName+" — unpack binds values only", "unpack")
	}
	if err := ValidateWordName(localName); err != nil {
		return err
	}
	if r.Defs.IsType(localName) {
		return r.AqlError("unpack_error", "unpack: name clash — "+localName+" is already a type", "unpack")
	}

	val, ok := get(srcKey)
	if !ok {
		if r.Check.IsActive() {
			val = NewCarrier(TAny)
		} else {
			return r.AqlError("unpack_error", "unpack: key "+srcKey+" not found in source", "unpack")
		}
	}
	_, err := installAndRecordDef(r, localName, val, pos)
	return err
}

// unpackHandler binds each name in args[0] to the matching entry of the
// map/record args[1] — `unpack [names] map`.
func unpackHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	names, err := RequireConcreteList(args[0], "unpack")
	if err != nil {
		return nil, err
	}
	get, _, err := unpackSource(args[1], r)
	if err != nil {
		return nil, err
	}
	for _, el := range names.Slice() {
		name := defName(el)
		if err := bindUnpackEntry(r, name, name, get, el.Pos); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// unpackAllHandler binds every key of the source — `unpack all map`.
// args[0] is the `all` keyword atom (only the literal `all` is accepted);
// args[1] is the source map/record.
func unpackAllHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	kw, err := args[0].AsConcreteAtom()
	if err != nil {
		return nil, fmt.Errorf("unpack: %w", err)
	}
	if kw != "all" {
		return nil, r.AqlError("unpack_error", "unpack: expected a name list, the keyword `all`, or a rename map, got "+kw, "unpack")
	}
	get, keys, err := unpackSource(args[1], r)
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		if err := bindUnpackEntry(r, k, k, get, args[0].Pos); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// unpackRenameHandler binds each source key to a chosen local name —
// `unpack {srcKey: localName …} map`. args[0] is the rename map (values
// kept un-evaluated via NoEvalMapArgs so the target names survive as
// bare words); args[1] is the source map/record.
func unpackRenameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	renames, err := RequireConcreteMap(args[0], "unpack")
	if err != nil {
		return nil, err
	}
	get, _, err := unpackSource(args[1], r)
	if err != nil {
		return nil, err
	}
	for _, srcKey := range renames.Keys() {
		target, _ := renames.Get(srcKey)
		localName := defName(target)
		if err := bindUnpackEntry(r, localName, srcKey, get, target.Pos); err != nil {
			return nil, err
		}
	}
	return nil, nil
}
