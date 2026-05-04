package engine

import (
	"fmt"
	"strings"
)

func RegisterTypeDef(r *Registry) {
	validateAndInstall := func(name string, body Value) error {
		if !isTypeBody(body) {
			return fmt.Errorf("type: body must be a type value (record, disjunct, type literal, typed list, or typed map), got %s", body.String())
		}
		if !IsCapitalisedName(name) {
			return fmt.Errorf("type %s: type names must start with a capital letter", name)
		}
		// Skip the known-parts conflict check when re-binding a name
		// that is already an active type — re-binding our own name
		// is shadowing, not a conflict. KnownTypeParts records every
		// part name we've ever installed and never shrinks; the
		// per-stack push handles the actual duplicate.
		if !r.HasType(name) {
			if err := ValidateTypeNameParts(name, r.KnownTypeParts); err != nil {
				return err
			}
		}
		// Refuse a type definition whose name already names a callable
		// or a def'd value. Type and def share a single source-level
		// namespace (the same Word resolves both), so allowing both
		// to bind the same name would silently change behaviour
		// depending on context. Type-vs-type re-binding IS allowed —
		// it shadows the previous type; `untype Foo` reverts.
		if r.Lookup(name) != nil {
			return fmt.Errorf("type %s: name clash — already a registered function", name)
		}
		if len(r.DefStacks[name]) > 0 {
			return fmt.Errorf("type %s: name clash — already a def'd value", name)
		}
		// All type bodies — fn-shape, predicate-fn, dependent scalar,
		// record, options, table, disjunct, typed list/map, object,
		// or plain type literal — live ONLY in r.Types. The previous
		// implementation mirrored non-fn bodies into DefStacks via
		// installDef so legacy resolution paths could find them; that
		// dual storage was the source of the ObjectType-rename drift
		// (§5.2 in TYPE-SYSTEM-REVIEW.md). With stepWord consulting
		// TopOfTypeStack ahead of DefStacks, the mirror is unnecessary
		// — and removing it eliminates an entire class of "the two
		// stacks got out of sync" bugs.
		//
		// ObjectType bodies need a name-path rebuild before installation
		// (Object, Object/Foo, Object/Foo/Bar) so MakeOrConvert and
		// related machinery can walk the inheritance chain. The rewrite
		// previously lived in installDef; it now happens here, keeping
		// the type-handling logic in one file.
		if body.IsObjectType() {
			info, _ := body.AsObjectType()
			if info.Parent != nil {
				info.Name = info.Parent.Name + "/" + name
			} else {
				info.Name = "Object/" + name
			}
			for _, p := range strings.Split(info.Name, "/") {
				r.KnownTypeParts[p] = true
			}
			body = NewObjectType(info)
		}
		r.PushType(name, body)
		// Register the new name parts as known. (Idempotent — already-
		// known parts stay known; this matters only for first-time
		// bindings of fresh names.)
		for _, p := range strings.Split(name, "/") {
			r.KnownTypeParts[p] = true
		}
		return nil
	}

	// Forward handler: "type foo number" → args=[foo(name), number(body)]
	// Forward precedence handles all orderings without infix signatures.
	typeHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		name := defName(args[0])
		body := args[1]
		if err := validateAndInstall(name, body); err != nil {
			return nil, err
		}
		return nil, nil
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:              "type",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:           []Type{TString, TAny},
				Handler:        typeHandler,
				Returns:        []Type{},
				RunInCheckMode: true,
			},
			{
				Args:           []Type{TAtom, TAny},
				QuoteArgs:      map[int]bool{0: true},
				Handler:        typeHandler,
				Returns:        []Type{},
				RunInCheckMode: true,
			},
		},
	})

	registerUntype(r)
}

// registerUntype installs `untype name` — the type counterpart of
// `undef`. Pops the most recent binding for the named type so a
// shadowed previous binding (if any) becomes active again, or the
// name becomes unbound if the stack empties.
//
// Sig is [TAtom/q] (forward, /q so a bare word is captured as the
// name without resolving to its type value first). Mirrors `undef`'s
// shape. Types live exclusively in r.Types — there's no DefStacks
// mirror to keep in sync.
func registerUntype(r *Registry) {
	untypeHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		name := defName(args[0])
		if !IsCapitalisedName(name) {
			return nil, fmt.Errorf("untype %s: type names must start with a capital letter", name)
		}
		if !r.PopType(name) {
			return nil, fmt.Errorf("untype %s: no such type binding", name)
		}
		return nil, nil
	}
	r.RegisterNativeFunc(NativeFunc{
		Name:              "untype",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:           []Type{TString},
				Handler:        untypeHandler,
				Returns:        []Type{},
				RunInCheckMode: true,
			},
			{
				Args:           []Type{TAtom},
				QuoteArgs:      map[int]bool{0: true},
				Handler:        untypeHandler,
				Returns:        []Type{},
				RunInCheckMode: true,
			},
		},
	})
}

// isTypeBody reports whether a value is a valid type definition body.
func isTypeBody(v Value) bool {
	// Type literal (Data==nil): number, string, boolean, any, etc.
	if v.Data == nil {
		return true
	}
	// Record type
	if v.IsRecordType() {
		return true
	}
	// Options type
	if v.IsOptionsType() {
		return true
	}
	// Table type
	if v.IsTableType() {
		return true
	}
	// Disjunct
	if v.IsDisjunct() {
		return true
	}
	// Typed list [:type]
	if v.IsTypedList() {
		return true
	}
	// Typed map {:type}
	if v.IsTypedMap() {
		return true
	}
	// Object type
	if v.IsObjectType() {
		return true
	}
	// Dependent scalar type (Integer gt 10, String lt "z", …)
	if v.IsDepScalar() {
		return true
	}
	// Function-signature type: a FnUndef carrying input + output sig
	// patterns and no body. Produced by `fn [[input] [output]]`. Used
	// as a structural function shape — `def n:Mapper f` requires f to
	// be a function whose signatures match the FnUndef pattern.
	if v.VType.Equal(TFnUndef) {
		return true
	}
	// Predicate type: a FnDef / Function whose body returns a Boolean.
	// Produced by `fn [x:Any Any [body]]`. Used as a *dependent* type
	// — `def n:Bbd value` calls the predicate against `value` and
	// installs the def iff the predicate returns true. The fn's
	// signature must take exactly one argument and return a Boolean
	// (or a value that converts to Boolean); enforcement happens in
	// the typed-def handler when it actually calls the predicate.
	if v.VType.Equal(TFnDef) || v.VType.Equal(TFunction) {
		return true
	}
	return false
}
