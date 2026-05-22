package eng

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Well-known types. Each is a pointer into the package-level Builtin
// TypeTable. Type identity is pointer equality; comparing T*-constants
// against each other or against runtime-constructed types just
// compares the Type pointers.
var (
	TAny            = mustType("Any")
	TNone           = mustType("None")
	TNever          = mustType("Never")
	TScalar         = mustType("Scalar")
	TString         = mustType("Scalar/String")
	TStringProper   = mustType("Scalar/String/ProperString")
	TStringEmpty    = mustType("Scalar/String/EmptyString")
	TNumber         = mustType("Scalar/Number")
	TInteger        = mustType("Scalar/Number/Integer")
	TDecimal        = mustType("Scalar/Number/Decimal")
	TBoolean        = mustType("Scalar/Boolean")
	TPath           = mustType("Scalar/Path")
	TNode           = mustType("Node")
	TIdeal          = mustType("Ideal")
	TList           = mustType("Node/List")
	TListArgs       = mustType("Node/List/Args")
	TMap            = mustType("Node/Map")
	TOptions        = mustType("Ideal/Options")
	TTable          = mustType("Ideal/Table")
	TRecord         = mustType("Ideal/Record")
	TAtom           = mustType("Scalar/Atom")
	TWord           = mustType("Word")
	TFunction       = mustType("Type/Function")
	TForward        = mustType("Word/__FW")
	TOpenParen      = mustType("Word/__OP")
	TCloseParen     = mustType("Word/__CP")
	TEnd            = mustType("Word/__ED")
	TParenExpr      = mustType("Word/__PE")
	TInterpString   = mustType("Word/__IS")
	TFnDef          = mustType("Word/__FN")
	TFnUndef        = mustType("Type/FunctionSignature")
	TReturnCheck    = mustType("Word/__RC")
	TDefCleanup     = mustType("Word/__IN/__DC")
	TDisjunct       = mustType("Type/Disjunct")
	TEnum           = mustType("Type/Disjunct/Enum")
	TMark           = mustType("Word/__MK")
	TMove           = mustType("Word/__MV")
	TModule         = mustType("Word/__MD")
	TInternal       = mustType("Word/__IN")
	TInspect        = mustType("Node/Map/Inspect")
	TObject         = mustType("Ideal/Object")
	TStore          = mustType("Ideal/Store")
	TStoreSystem    = mustType("Ideal/Store/System")
	TArray          = mustType("Ideal/Array")
	TResource       = mustType("Ideal/Object/Resource")
	TResourceEntity = mustType("Ideal/Object/Resource/Entity")
	// TFetchFunction / TFetchRequest / TFetchResponse moved to
	// lang/go/native/fetch.go (Step 8 migration); registered via
	// RegisterExternalBuiltin at lang/go/native package init.
	TError      = mustType("Ideal/Error")
	TType       = mustType("Type")
	TScalarType = mustType("Type/ScalarType")
	TNodeType   = mustType("Type/NodeType")
	TIdealType  = mustType("Type/IdealType")
	// Scalar/Time and descendants moved to
	// lang/go/engine/native_temporal.go (Step 8). They live in
	// lang/go/engine (not lang/go/internal/nativemod/time) because
	// date-arithmetic handlers in lang/go/engine/native_math.go
	// reference these types at package-init time — colocating
	// avoids the import-cycle constraint.
	// TMatrix moved to lang/go/internal/nativemod/matrix.go (Step 8).
	// TTimeout moved to lang/go/engine/native_misc.go (Step 8).
	TDependent  = mustType("Type/Dependent")
	TDepInteger = mustType("Type/Dependent/DepInteger")
	// TInterval moved to lang/go/engine/native_misc.go (Step 8).
)

// typeNames is the user-facing name → Type map for bare-word type
// resolution. Built at init from the Builtin TypeTable; refreshed
// whenever a new externally-registered builtin is added via
// RegisterExternalBuiltin. The parser and engine consult this map
// to resolve bare type-name words (e.g. `Integer`, `Date`, plugin-
// supplied names) to their *Type.
var typeNames = buildTypeNames()

func buildTypeNames() map[string]*Type {
	m := make(map[string]*Type, len(Builtin.byName))
	for name, def := range Builtin.byName {
		if def == nil {
			continue
		}
		m[name] = def
	}
	return m
}

// refreshTypeNames rebuilds the typeNames snapshot. Called by
// RegisterExternalBuiltin so freshly-installed types are immediately
// resolvable by bare-name lookup in the parser.
func refreshTypeNames() {
	typeNames = buildTypeNames()
}

// TypeNameTable returns the canonical mapping of all well-known type
// names to their Type. Used by both the parser and engine.
func TypeNameTable() map[string]*Type {
	return typeNames
}

// TypeNameByID returns the canonical user-facing name for a Type ID
// (e.g. the ID for "Type/FunctionSignature" → "FunctionSignature").
// Returns the empty string if no entry exists or the type is internal.
func TypeNameByID(id string) string {
	def := Builtin.LookupByID(id)
	if def == nil || def.IsInternal {
		return ""
	}
	// Only return a name for types that participate in user-facing
	// name resolution (present in byName).
	if _, ok := Builtin.byName[def.Name]; !ok {
		return ""
	}
	return def.Name
}

// mustType is used only for well-known type constants at init time.
// It panics on invalid paths — acceptable because these are compile-time
// constants whose correctness is verified by tests.
func mustType(path string) *Type {
	return mustBuiltinType(path)
}

// NewType resolves a slash-separated path to a builtin *Type, e.g.
// "String/ProperString". Short names are auto-expanded to their full
// hierarchy path: "String/ProperString" becomes
// "Scalar/String/ProperString", etc. Every alphabetic part must begin
// with an uppercase letter; lowercase is an error.
//
// NewType resolves *builtin* paths only. User-defined types must be
// minted via TypeTable.MintType — an otherwise-unknown path is a hard
// error. Test scaffolding that needs to synthesise arbitrary type
// hierarchies should use MintTestType, which mints intermediate
// *Types under the builtin root as needed.
func NewType(path string) (*Type, error) {
	parts := strings.Split(path, "/")
	for _, p := range parts {
		r, _ := utf8.DecodeRuneInString(p)
		if unicode.IsLetter(r) && !unicode.IsUpper(r) {
			return nil, fmt.Errorf("aql: type part %q in %q must start with an uppercase letter", p, path)
		}
	}

	// Auto-expand short names to full hierarchy paths via the Builtin
	// table. If the first part is already a root, no expansion needed.
	if _, isRoot := Builtin.bypath[parts[0]]; !isRoot {
		if fullPrefix, ok := Builtin.ExpandShortName(parts[0]); ok {
			expanded := fullPrefix
			if len(parts) > 1 {
				expanded += "/" + strings.Join(parts[1:], "/")
			}
			parts = strings.Split(expanded, "/")
		}
	}

	fullPath := strings.Join(parts, "/")
	if def := Builtin.bypath[fullPath]; def != nil {
		return def, nil
	}
	return nil, fmt.Errorf("aql: unknown type %q", fullPath)
}

// ResolveTypePath attempts to resolve a slash-separated path to a
// known builtin Type. Returns the Type and true only if the path
// matches a registered Builtin entry — transient Type values minted
// by NewType for unknown paths do NOT count. Single-part names are
// resolved via the parser's TypeNameTable bare-word lookup instead.
func ResolveTypePath(name string) (*Type, bool) {
	if !strings.Contains(name, "/") {
		return nil, false
	}
	t, err := NewType(name)
	if err != nil {
		return nil, false
	}
	if t != nil && Builtin.bypath[t.Path()] == t {
		return t, true
	}
	return nil, false
}

// Matches reports whether this type satisfies the given pattern.
//   - "Any" pattern matches everything.
//   - A child matches a parent: Scalar/String/ProperString matches Scalar/String.
//   - A parent does NOT match a child: Scalar/String does not match Scalar/String/ProperString.
//   - A Type/Dependent/Dep<X> path is treated as a subtype of <X> and any
//     of <X>'s lattice ancestors. The Dependent branch lives under its own
//     root for clear separation, but dependent values must satisfy any slot
//     expecting the underlying base. Per-value satisfaction (does 5 lie in
//     [10, ∞)?) is handled at Unify time; this method answers the type-level
//     question only.
func (t *Type) Matches(pattern *Type) bool {
	// Empty pattern (nil Type) is a vacuous match — preserves the
	// old PathSubtype semantics where a zero-iteration prefix loop
	// returned true. Dispatcher / coercion code relies on this.
	if pattern == nil {
		return true
	}
	if pattern == TAny {
		return true
	}
	if t == nil {
		return false
	}
	if t.IsAncestor(pattern) {
		return true
	}
	// Dependent-scalar override: a Dep<X> value satisfies any slot
	// typed as X (its underlying base). The BaseType pointer on the
	// *Type is populated at registration via builtinDecl.BasePath;
	// the historical hardcoded leaf-name switch in depscalar.go is
	// now a per-Type field (Step 9 of TYPE-DECOUPLING.0.md).
	if t.BaseType != nil && t.BaseType.Matches(pattern) {
		return true
	}
	return false
}

// PathSubtype reports whether t is a strict path-prefix subtype of
// pattern. With ID-based identity, this is equivalent to the ancestry
// walk used by Matches — no `Any` bolt-on, no Dep<Leaf> base bolt-on.
//
// `t.PathSubtype(t)` is always true (identity is a subtype of itself).
func (t *Type) PathSubtype(pattern *Type) bool {
	if t == nil || pattern == nil {
		return false
	}
	return t.IsAncestor(pattern)
}

// Specificity returns the depth of the type. More ancestry = more specific.
func (t *Type) Specificity() int {
	n := 0
	for d := t; d != nil; d = d.Parent {
		n++
	}
	return n
}

// Leaf returns the last part of the type path.
// For example, "Object/Fetch/Request" returns "Request".
func (t *Type) Leaf() string {
	if t == nil {
		return ""
	}
	return t.Name
}

// IsSubtypeOf reports whether t is a strict subtype of parent.
// For example: Scalar/String/ProperString is a subtype of Scalar/String.
// A type is NOT a subtype of itself.
func (t *Type) IsSubtypeOf(parent *Type) bool {
	if t == nil || parent == nil || t == parent {
		return false
	}
	return t.IsAncestor(parent)
}

// Equal reports whether two types are identical. Identity is pointer
// equality on Type.
func (t *Type) Equal(other *Type) bool {
	return t == other
}

// MetatypeFor returns the metatype for a given type.
// Scalar subtypes → TScalarType, Node subtypes → TNodeType,
// Object subtypes → TObjectType, everything else → TType.
//
// The mapping is driven by *Type.Metatype, set at registration via
// builtinDecl.MetatypePath on the three anchor roots (Scalar, Node,
// Object). Descendants of those roots inherit the anchor by walking
// up the Parent chain. Step 9 of TYPE-DECOUPLING.0.md replaced the
// historical root-name switch with this field-driven lookup so a
// new root with its own metatype can be added by declaring its
// MetatypePath, no central function edit required.
func MetatypeFor(t *Type) *Type {
	if t == nil || t.Parent == nil {
		return TType
	}
	for d := t; d != nil; d = d.Parent {
		if d.Metatype != nil {
			return d.Metatype
		}
	}
	return TType
}

// IsMetaType reports whether t is in the Type/* metatype hierarchy.
func IsMetaType(t *Type) bool {
	return t != nil && t.Root() == TType
}

// ValidateTypeNameParts checks that a type name (slash-separated) does not
// reuse any part that is already known per the isKnown callback. Returns
// an error on the first conflict.
func ValidateTypeNameParts(name string, isKnown func(string) bool) error {
	parts := strings.Split(name, "/")
	for _, p := range parts {
		if isKnown(p) {
			return fmt.Errorf("type: name part %q in %q conflicts with an existing type name", p, name)
		}
	}
	return nil
}

// IsCapitalisedName reports whether name starts with an ASCII upper-case
// letter. The naming rule is: type names start with a capital, def names
// don't. Empty names are not capitalised.
func IsCapitalisedName(name string) bool {
	if name == "" {
		return false
	}
	c := name[0]
	return c >= 'A' && c <= 'Z'
}
