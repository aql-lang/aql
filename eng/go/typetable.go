package eng

import (
	"fmt"
	"strings"
)

// OriginKind classifies where a *Type was registered. Every *Type
// is either seeded at init from the package-level builtinDecls list
// (OriginBuiltin) or minted at runtime via TypeTable.MintType — the
// path the `type` word and the anonymous `object {…}` handler take
// when introducing new identities (OriginUserDef).
type OriginKind uint8

const (
	// OriginBuiltin is set on every *Type seeded from builtinDecls
	// into the package-level Builtin table. Builtins have a stable
	// FixedID and never go away. Value-tagged subtypes minted by
	// NewType for paths like Scalar/Number/Integer/42 are also
	// OriginBuiltin — they're parametric instances of a builtin
	// parent, not user declarations.
	OriginBuiltin OriginKind = iota
	// OriginUserDef is set on every *Type minted by TypeTable.MintType
	// — the named `type Foo …` flow and the anonymous `object {…}`
	// constructor. Each mint produces a fresh *Type with a unique ID;
	// named types are then registered via Bind, anonymous ones are
	// not.
	OriginUserDef
)

// String returns a short human-readable label for the origin.
func (o OriginKind) String() string {
	switch o {
	case OriginBuiltin:
		return "builtin"
	case OriginUserDef:
		return "userdef"
	}
	return "unknown"
}

// Type is an alias for Value. The type lattice and the value space
// are one structure: a "type" is a Value used as a lattice node —
// its Parent is its supertype, its Behavior drives is/format/equal,
// its Name/FixedID/Rank carry lattice metadata. The full field set
// and the kernel/value duality are documented on the Value struct
// in value.go.
//
// The alias keeps the *Type spelling working at the call sites that
// traffic in lattice nodes; *Type and *Value are the same type.
// Type identity is pointer equality; builtins are seeded at init
// from builtinDecls, and MintType mints fresh identities at runtime.
type Type = Value

// IsNative reports whether t is a built-in type seeded at init from
// the package-level builtinDecls list. Returns false for user-defined
// types installed via the `type` word and for transient pool entries
// minted by NewType for unknown paths. Safe to call on a nil receiver.
func (t *Type) IsNative() bool {
	return t != nil && t.Origin == OriginBuiltin
}

// Path returns the slash-separated path by walking up the parent chain.
func (t *Type) Path() string {
	if t == nil {
		return ""
	}
	if t.Parent == nil {
		return t.Name
	}
	return t.Parent.Path() + "/" + t.Name
}

// Root returns the top of the ancestry chain.
func (t *Type) Root() *Type {
	if t == nil {
		return nil
	}
	for t.Parent != nil {
		t = t.Parent
	}
	return t
}

// IsAncestor reports whether ancestor lies on t's parent chain (or is
// t). Comparison is by lattice identity (Equal) so a by-value
// type-literal copy still recognises its own ancestors.
func (t *Type) IsAncestor(ancestor *Type) bool {
	for x := t; x != nil; x = x.Parent {
		if x.Equal(ancestor) {
			return true
		}
	}
	return false
}

// TypeTable is the canonical catalogue of types. Builtin is the
// package-level table; per-Registry dynamic tables extend it via
// MintType.
//
// Post the TYPE-UNIFORM Phase 4 collapse the TypeTable is purely the
// type *lattice* — it no longer owns a dynamic binding stack. Type
// bindings installed by a capitalised `def` live in the Registry's
// single DefTable; this table only mints lattice identities
// (MintType), indexes them by ID (byID), and keeps the static builtin
// name index (byName).
type TypeTable struct {
	byID      map[string]*Type
	byName    map[string]*Type  // builtin name → Type (static; dynamic bindings live in DefTable)
	parts     map[string]bool   // every Part name used by a registered type
	bypath    map[string]*Type  // builtin-only path index (dynamic types can collide on path)
	rootSet   map[string]bool   // roots, for fast IsRoot checks
	leafIndex map[string]string // builtin leaf-name → full path; "" if ambiguous
	seq       int               // counter for minting dynamic IDs
}

// dynamicIDBase is the starting point for minted IDs, chosen well above
// any builtin FixedID so dynamic IDs never collide with builtins.
const dynamicIDBase = 0x10000000

// Lookup returns the Type for a builtin path, or nil if none.
// Dynamic types are NOT in this index — use Registry.LookupTypeName
// for shadow-aware resolution and LookupByID for direct identity lookup.
func (tt *TypeTable) Lookup(path string) *Type {
	if tt == nil {
		return nil
	}
	return tt.bypath[path]
}

// LookupByID returns the Type for a canonical ID, or nil if none.
func (tt *TypeTable) LookupByID(id string) *Type {
	if tt == nil || id == "" {
		return nil
	}
	return tt.byID[id]
}

// LookupBuiltinByName returns the builtin Type registered under a
// user-facing short name, or nil. Dynamic type bindings are NOT here —
// they live in the Registry's DefTable; use Registry.LookupTypeName
// for shadow-aware resolution across both.
func (tt *TypeTable) LookupBuiltinByName(name string) *Type {
	if tt == nil {
		return nil
	}
	return tt.byName[name]
}

// IsRoot reports whether part is a top-level root name (Scalar, Node, …).
func (tt *TypeTable) IsRoot(part string) bool {
	if tt == nil {
		return false
	}
	return tt.rootSet[part]
}

// KnownPart reports whether part appears in any registered type's path.
func (tt *TypeTable) KnownPart(part string) bool {
	if tt == nil {
		return false
	}
	return tt.parts[part]
}

// NewDynamicTypeTable returns an empty TypeTable for per-Registry use.
// Builtins are NOT pre-seeded; lookups for builtins go through the
// package-level Builtin table at call sites that need them.
func NewDynamicTypeTable() *TypeTable {
	return &TypeTable{
		byID:   make(map[string]*Type),
		byName: make(map[string]*Type),
		parts:  make(map[string]bool),
	}
}

// mintID generates a fresh ID for a dynamically registered Type.
// The prefix mirrors the builtin convention (S_/N_/W_/T_) so dynamic
// IDs carry the same root-category signal as builtins.
func (tt *TypeTable) mintID(parent *Type) string {
	tt.seq++
	prefix := "T_"
	if parent != nil {
		switch parent.Root().Name {
		case "Scalar":
			prefix = "S_"
		case "Node":
			prefix = "N_"
		case "Word":
			prefix = "W_"
		}
	}
	return fmt.Sprintf("%s%012x", prefix, dynamicIDBase+tt.seq)
}

// MintType creates a fresh *Type with Origin=OriginUserDef and
// registers it in byID. The returned *Type is unbound — call Bind to
// associate it with a user-facing name. Callers typically mint, then
// construct a body Value using the returned *Type as its Parent, then
// Bind. Anonymous types (e.g. `object {…}` not installed by name)
// skip the Bind step and just keep the *Type as the Value's identity.
//
// Behavior defaults to DefaultBehavior. Callers needing custom
// dispatch construct the *Type via this path then set def.Behavior
// before exposing it, or use MintTypeWithBehavior.
func (tt *TypeTable) MintType(name string, parent *Type) *Type {
	def := &Type{
		Name:     name,
		Parent:   parent,
		Origin:   OriginUserDef,
		Behavior: DefaultBehavior,
	}
	// A user type inherits its parent's unified Rank — it gets no
	// positional slot; compareTypes breaks same-Rank ties by name/id.
	if parent != nil {
		def.Rank = parent.Rank
	}
	def.ID = tt.mintID(parent)
	tt.byID[def.ID] = def
	return def
}

// RegisterExternalBuiltin installs a non-kernel-declared "builtin-
// class" type from outside the eng package — host modules
// (lang/go/modules/time, lang/go/native/fetch, plugin packages,
// etc.) that own a type the kernel doesn't need to know about by
// name. Conceptually equivalent to a builtinDecls row, but supplied
// at runtime by the owning module.
//
// FixedID allocation policy: each module reserves a stable per-module
// range so cross-version ID stability survives reorderings and
// plugin loadings. Reserved ranges:
//
//	  100-999    eng-internal future-builtins
//	 1000-1999   lang/go/modules/time   (Date, DateTime, Instant, …)
//	 2000-2999   lang/go/modules/matrix (Matrix)
//	 3000-3999   lang/go/native/fetch              (Fetch, Request, Response)
//	 4000-4999   lang/go/engine (Timeout, Interval)
//	 5000-9999   reserved for future kernel use
//	10000+       host / third-party plugin types
//
// Callers register at module init (e.g. modules.RegisterTypes(r))
// and capture the returned *Type into a package-level variable. The
// kernel's dispatch path consults the type's Behavior — no special
// case for "external vs builtin" exists at runtime.
//
// Validates the path is well-formed (every part starts with [A-Z]),
// the parent path is registered, and the FixedID is unused. Returns
// the minted *Type on success.
func (tt *TypeTable) RegisterExternalBuiltin(path string, fixedID int, behavior TypeBehavior) (*Type, error) {
	parts := strings.Split(path, "/")
	if len(parts) == 0 || path == "" {
		return nil, fmt.Errorf("RegisterExternalBuiltin: empty path")
	}
	for _, p := range parts {
		if p == "" {
			return nil, fmt.Errorf("RegisterExternalBuiltin: invalid path %q (empty part)", path)
		}
		c := p[0]
		if c < 'A' || c > 'Z' {
			if !strings.HasPrefix(p, "__") {
				return nil, fmt.Errorf("RegisterExternalBuiltin: invalid path %q (part %q must start with [A-Z])", path, p)
			}
		}
	}

	var parent *Type
	if len(parts) > 1 {
		parentPath := strings.Join(parts[:len(parts)-1], "/")
		parent = tt.bypath[parentPath]
		if parent == nil {
			return nil, fmt.Errorf("RegisterExternalBuiltin: parent %q not registered for %q", parentPath, path)
		}
	}

	if existing := tt.bypath[path]; existing != nil {
		return nil, fmt.Errorf("RegisterExternalBuiltin: path %q already registered", path)
	}

	id := formatFixedID(path, fixedID)
	if existing, dup := tt.byID[id]; dup {
		return nil, fmt.Errorf("RegisterExternalBuiltin: FixedID %d for %q collides with %q", fixedID, path, existing.Path())
	}

	if behavior == nil {
		behavior = DefaultBehavior
	}

	def := &Type{
		ID:       id,
		Name:     parts[len(parts)-1],
		Parent:   parent,
		FixedID:  fixedID,
		Origin:   OriginBuiltin,
		Behavior: behavior,
	}
	// External builtins inherit the parent's unified Rank (no
	// positional slot — see builtinDecls).
	if parent != nil {
		def.Rank = parent.Rank
	}
	tt.byID[id] = def
	tt.bypath[path] = def
	if parent == nil {
		tt.rootSet[path] = true
	}
	tt.byName[def.Name] = def
	for _, p := range parts {
		tt.parts[p] = true
	}
	if existing, dup := tt.leafIndex[def.Name]; dup {
		if existing != "" {
			tt.leafIndex[def.Name] = ""
		}
	} else {
		tt.leafIndex[def.Name] = path
	}
	// Refresh the parser's bare-name lookup snapshot so the newly-
	// registered type is resolvable by source-text references like
	// `Foo`. Only the package-level Builtin table feeds typeNames;
	// per-Registry dynamic tables do not.
	if tt == Builtin {
		refreshTypeNames()
	}
	return def, nil
}

// MintTypeWithBehavior is MintType plus a custom TypeBehavior. Used
// by registration paths that want to install a domain-specific
// Behavior at mint time (predicate types, dependent scalars, plugin
// types). A nil behavior falls back to DefaultBehavior.
func (tt *TypeTable) MintTypeWithBehavior(name string, parent *Type, behavior TypeBehavior) *Type {
	def := tt.MintType(name, parent)
	if behavior != nil {
		def.Behavior = behavior
	}
	return def
}

// Retire removes a dynamically-minted type from the ID index. Called
// by `undef` when a type binding is popped from the Registry's single
// DefTable, so the retired identity no longer resolves via LookupByID.
func (tt *TypeTable) Retire(def *Type) {
	if tt == nil || def == nil {
		return
	}
	delete(tt.byID, def.ID)
}

// Clone returns a deep copy of tt — used for snapshot/restore around
// predicate sandbox boundaries. Type pointers are shared (defs are
// immutable once minted); only the stacks themselves are duplicated.
func (tt *TypeTable) Clone() *TypeTable {
	if tt == nil {
		return nil
	}
	nt := &TypeTable{
		byID:   make(map[string]*Type, len(tt.byID)),
		byName: make(map[string]*Type, len(tt.byName)),
		parts:  make(map[string]bool, len(tt.parts)),
		seq:    tt.seq,
	}
	for k, v := range tt.byID {
		nt.byID[k] = v
	}
	for k, v := range tt.byName {
		nt.byName[k] = v
	}
	for k, v := range tt.parts {
		nt.parts[k] = v
	}
	return nt
}

// builtinDecl describes one builtin type. The declarative list below
// is the SINGLE SOURCE OF TRUTH for all builtin types — IDs, parents,
// user-facing visibility, everything.
type builtinDecl struct {
	Path         string
	FixedID      int
	IsInternal   bool   // true for Word/__XX runtime markers
	Alias        string // optional friendly short name for ExpandShortName (e.g. "Paren" → Word/__OP)
	BasePath     string // for Type/Dependent/Dep<X> types: the path of the underlying scalar (Step 9)
	MetatypePath string // for root types whose descendants share a metatype anchor (Scalar→Type/ScalarType, …)
	Rank         int    // unified lattice rank — see builtinDecls
}

// builtinDecls lists every builtin type. Parent-first ordering is
// required so registerBuiltin can wire Parent pointers as it walks.
//
// FixedID values are stable across runs and must not change once
// assigned — they appear in serialized IDs. New types must use a fresh
// number, never recycle an old one.
//
// Rank is the unified lattice rank: a single integer giving the total
// order CompareValues / compareTypes use for cross-type ordering. It is
// positional — a type's Rank is its parent's Rank plus a depth-scaled
// offset, so a parent always ranks below its whole subtree and sibling
// order is least-to-most complex:
//
//	depth 0  roots          1e10 bands (Any/None/Never share band 1)
//	depth 1  branch kinds   +1e8 per sibling
//	depth 2  refinements    +1e7 per sibling   (Word markers: +1e3)
//
// User types (MintType) and external builtins (RegisterExternalBuiltin)
// do not get a positional slot — they inherit the parent's Rank, and
// compareTypes breaks the resulting ties by name/id. Max rank ≈ 6e10,
// far under the int64 ceiling.
var builtinDecls = []builtinDecl{
	// Roots. Any/None/Never are childless degenerate roots packed into
	// the first 1e10 Rank band; the five structural roots take a 1e10
	// band each. See the unified-Rank scheme on builtinDecl.Rank.
	{Path: "Any", FixedID: 1, Rank: 11_000_000_000},
	{Path: "None", FixedID: 2, Rank: 12_000_000_000},
	{Path: "Never", FixedID: 61, Rank: 13_000_000_000},
	{Path: "Scalar", FixedID: 3, Rank: 20_000_000_000, MetatypePath: "Type/ScalarType"},
	{Path: "Node", FixedID: 11, Rank: 30_000_000_000, MetatypePath: "Type/NodeType"},
	{Path: "Ideal", FixedID: 48, Rank: 40_000_000_000, MetatypePath: "Type/IdealType"},
	{Path: "Word", FixedID: 17, Rank: 50_000_000_000},
	{Path: "Type", FixedID: 39, Rank: 60_000_000_000},

	// Scalar branch — children ordered least-to-most complex.
	{Path: "Scalar/Atom", FixedID: 18, Rank: 20_100_000_000},
	{Path: "Scalar/Boolean", FixedID: 10, Rank: 20_200_000_000},
	{Path: "Scalar/Number", FixedID: 7, Rank: 20_300_000_000},
	{Path: "Scalar/Number/Integer", FixedID: 8, Rank: 20_310_000_000},
	{Path: "Scalar/Number/Decimal", FixedID: 9, Rank: 20_320_000_000},
	{Path: "Scalar/String", FixedID: 4, Rank: 20_400_000_000},
	{Path: "Scalar/String/EmptyString", FixedID: 6, Rank: 20_410_000_000},
	{Path: "Scalar/String/ProperString", FixedID: 5, Rank: 20_420_000_000},
	{Path: "Scalar/Path", FixedID: 47, Rank: 20_500_000_000},
	// Scalar/Time and descendants live in lang/go/native/native_temporal.go.

	// Node branch.
	{Path: "Node/List", FixedID: 12, Rank: 30_100_000_000},
	{Path: "Node/List/Args", FixedID: 13, Rank: 30_110_000_000},
	{Path: "Node/Map", FixedID: 14, Rank: 30_200_000_000},
	{Path: "Node/Map/Inspect", FixedID: 31, Rank: 30_210_000_000},

	// Ideal branch — the structural type-kinds (Object, Array, Record,
	// Options, Error, Store, Table) are direct children of Ideal: peer
	// kinds. Resource/Entity are genuine object types (an Object ←
	// Resource ← Entity inheritance chain), so they stay under Object.
	// External modules graft Tensor / Timeout / Fetch / … on as further
	// Ideal/* kinds via RegisterExternalBuiltin.
	{Path: "Ideal/Object", FixedID: 30, Rank: 40_100_000_000},
	{Path: "Ideal/Object/Resource", FixedID: 36, Rank: 40_110_000_000},
	{Path: "Ideal/Object/Resource/Entity", FixedID: 37, Rank: 40_111_000_000},
	{Path: "Ideal/Array", FixedID: 44, Rank: 40_200_000_000},
	{Path: "Ideal/Record", FixedID: 16, Rank: 40_300_000_000},
	{Path: "Ideal/Options", FixedID: 38, Rank: 40_400_000_000},
	{Path: "Ideal/Error", FixedID: 45, Rank: 40_500_000_000},
	{Path: "Ideal/Store", FixedID: 42, Rank: 40_600_000_000},
	{Path: "Ideal/Store/System", FixedID: 43, Rank: 40_610_000_000},
	{Path: "Ideal/Table", FixedID: 15, Rank: 40_700_000_000},

	// Word branch — Word/__XX entries are internal runtime markers,
	// packed at 1e3 Rank spacing. They expose friendly short-name
	// aliases (e.g. "Paren" → Word/__OP) so ResolveTypeName / NewType
	// can resolve them by their lang-level label.
	{Path: "Word/__FW", FixedID: 21, IsInternal: true, Alias: "Forward", Rank: 50_100_000_000},
	{Path: "Word/__OP", FixedID: 22, IsInternal: true, Alias: "Paren", Rank: 50_100_001_000},
	{Path: "Word/__CP", FixedID: 72, IsInternal: true, Alias: "CloseParen", Rank: 50_100_002_000},
	{Path: "Word/__ED", FixedID: 73, IsInternal: true, Alias: "End", Rank: 50_100_003_000},
	{Path: "Word/__PE", FixedID: 63, IsInternal: true, Rank: 50_100_004_000},
	{Path: "Word/__IS", FixedID: 51, IsInternal: true, Rank: 50_100_005_000},
	{Path: "Word/__FN", FixedID: 23, IsInternal: true, Alias: "Fndef", Rank: 50_100_006_000},
	{Path: "Word/__RC", FixedID: 25, IsInternal: true, Alias: "Returncheck", Rank: 50_100_007_000},
	{Path: "Word/__MK", FixedID: 27, IsInternal: true, Alias: "Mark", Rank: 50_100_008_000},
	{Path: "Word/__MV", FixedID: 28, IsInternal: true, Alias: "Move", Rank: 50_100_009_000},
	{Path: "Word/__MD", FixedID: 29, IsInternal: true, Alias: "Module", Rank: 50_100_010_000},
	{Path: "Word/__IN", FixedID: 20, IsInternal: true, Rank: 50_100_011_000},
	{Path: "Word/__IN/__DC", FixedID: 64, IsInternal: true, Rank: 50_100_011_001},

	// Type (metatype) branch.
	{Path: "Type/Function", FixedID: 19, Rank: 60_100_000_000},
	{Path: "Type/FunctionSignature", FixedID: 24, Rank: 60_200_000_000},
	{Path: "Type/Disjunct", FixedID: 26, Rank: 60_300_000_000},
	{Path: "Type/Disjunct/Enum", FixedID: 62, Rank: 60_310_000_000},
	{Path: "Type/ScalarType", FixedID: 40, Rank: 60_400_000_000},
	{Path: "Type/NodeType", FixedID: 41, Rank: 60_500_000_000},
	{Path: "Type/IdealType", FixedID: 46, Rank: 60_600_000_000},
	{Path: "Type/Dependent", FixedID: 65, Rank: 60_700_000_000},
	{Path: "Type/Dependent/DepAtom", FixedID: 71, BasePath: "Scalar/Atom", Rank: 60_710_000_000},
	{Path: "Type/Dependent/DepBoolean", FixedID: 70, BasePath: "Scalar/Boolean", Rank: 60_720_000_000},
	{Path: "Type/Dependent/DepNumber", FixedID: 68, BasePath: "Scalar/Number", Rank: 60_730_000_000},
	{Path: "Type/Dependent/DepInteger", FixedID: 66, BasePath: "Scalar/Number/Integer", Rank: 60_740_000_000},
	{Path: "Type/Dependent/DepDecimal", FixedID: 67, BasePath: "Scalar/Number/Decimal", Rank: 60_750_000_000},
	{Path: "Type/Dependent/DepString", FixedID: 69, BasePath: "Scalar/String", Rank: 60_760_000_000},
}

// Builtin is the package-level TypeTable holding every builtin type.
// It is populated once at init from builtinDecls and is read-only
// thereafter — per-Registry dynamic tables extend it via PushType.
var Builtin = newBuiltinTypeTable()

func newBuiltinTypeTable() *TypeTable {
	tt := &TypeTable{
		byID:      make(map[string]*Type, len(builtinDecls)),
		byName:    make(map[string]*Type),
		parts:     make(map[string]bool),
		bypath:    make(map[string]*Type, len(builtinDecls)),
		rootSet:   make(map[string]bool),
		leafIndex: make(map[string]string),
	}
	for _, d := range builtinDecls {
		tt.registerBuiltin(d)
	}
	// Post-pass: wire Metatype fields. Roots that anchor a metatype
	// (Scalar→ScalarType, Node→NodeType, Object→IdealType) resolve
	// their MetatypePath here, after all decls have been registered.
	// Every descendant of a metatype-bearing root inherits its
	// ancestor's Metatype by walking up — done lazily by MetatypeFor.
	for _, d := range builtinDecls {
		if d.MetatypePath == "" {
			continue
		}
		def := tt.bypath[d.Path]
		if def == nil {
			panic(fmt.Sprintf("typetable: post-pass cannot find %q", d.Path))
		}
		mt := tt.bypath[d.MetatypePath]
		if mt == nil {
			panic(fmt.Sprintf("typetable: metatype %q not registered for %q", d.MetatypePath, d.Path))
		}
		def.Metatype = mt
	}
	return tt
}

func (tt *TypeTable) registerBuiltin(d builtinDecl) {
	parts := strings.Split(d.Path, "/")
	var parent *Type
	if len(parts) > 1 {
		parentPath := strings.Join(parts[:len(parts)-1], "/")
		parent = tt.bypath[parentPath]
		if parent == nil {
			panic(fmt.Sprintf("typetable: parent %q not registered before %q (declare parents first in builtinDecls)", parentPath, d.Path))
		}
	}
	id := formatFixedID(d.Path, d.FixedID)
	if existing, dup := tt.byID[id]; dup {
		panic(fmt.Sprintf("typetable: duplicate FixedID %d for %q (already used by %q)", d.FixedID, d.Path, existing.Path()))
	}
	def := &Type{
		ID:         id,
		Name:       parts[len(parts)-1],
		Parent:     parent,
		FixedID:    d.FixedID,
		Rank:       d.Rank,
		IsInternal: d.IsInternal,
		Origin:     OriginBuiltin,
		Behavior:   DefaultBehavior,
	}
	if d.BasePath != "" {
		base := tt.bypath[d.BasePath]
		if base == nil {
			panic(fmt.Sprintf("typetable: base %q not registered before %q (declare base types first in builtinDecls)", d.BasePath, d.Path))
		}
		def.BaseType = base
	}
	tt.byID[id] = def
	tt.bypath[d.Path] = def
	if parent == nil {
		tt.rootSet[d.Path] = true
	}
	if !d.IsInternal {
		tt.byName[def.Name] = def
	}
	for _, p := range parts {
		tt.parts[p] = true
	}
	if existing, dup := tt.leafIndex[def.Name]; dup {
		// Ambiguous leaf name — mark with "" so ExpandShortName won't expand.
		if existing != "" {
			tt.leafIndex[def.Name] = ""
		}
	} else {
		tt.leafIndex[def.Name] = d.Path
	}
	if d.Alias != "" {
		tt.leafIndex[d.Alias] = d.Path
	}
}

// ExpandShortName returns the full builtin path for a short leaf name
// (e.g. "Integer" → "Scalar/Number/Integer"). Returns ok=false if the
// name is unknown or maps to multiple builtin paths.
func (tt *TypeTable) ExpandShortName(short string) (string, bool) {
	if tt == nil {
		return "", false
	}
	p, ok := tt.leafIndex[short]
	if !ok || p == "" {
		return "", false
	}
	return p, true
}

// formatFixedID formats a fixed numeric ID with the prefix derived
// from the path's root part. Output is 14 chars: "<prefix>_<12 hex>".
func formatFixedID(path string, num int) string {
	root := path
	if i := strings.IndexByte(path, '/'); i >= 0 {
		root = path[:i]
	}
	prefix := "T_"
	switch root {
	case "Scalar":
		prefix = "S_"
	case "Node":
		prefix = "N_"
	case "Word":
		prefix = "W_"
	}
	return fmt.Sprintf("%s%012x", prefix, num)
}

// MintTestType is a test-only helper that mints a *Type from a
// slash-separated path, walking from the builtin root where possible
// and minting intermediate *Types as needed. Used by lattice / Matches /
// Specificity tests that need synthetic type hierarchies; production
// code goes through NewType (strict — unknown paths error) or
// TypeTable.MintType (explicit name + parent).
//
// Short-name first parts are auto-expanded the same way NewType does
// it, so MintTestType("Number/Float") attaches under the builtin
// Scalar/Number rather than under a fresh top-level "Number".
//
// Minted entries are cached per path string so repeated calls return
// the same *Type. Origin is OriginUserDef. NOT for use outside tests.
func MintTestType(path string) *Type {
	if def := testTypePool[path]; def != nil {
		return def
	}
	parts := strings.Split(path, "/")
	// Auto-expand short-name first parts via the Builtin leaf index
	// (mirrors NewType so test paths under "Number" land beneath
	// Scalar/Number).
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
	// If the fully-expanded path is itself a builtin, return that — no
	// need to mint a separate test type for known types.
	if def := Builtin.bypath[fullPath]; def != nil {
		testTypePool[path] = def
		return def
	}
	var parent *Type
	if len(parts) > 1 {
		parentPath := strings.Join(parts[:len(parts)-1], "/")
		if p := Builtin.bypath[parentPath]; p != nil {
			parent = p
		} else {
			parent = MintTestType(parentPath)
		}
	}
	testTypeSeq++
	prefix := "T_"
	if parent != nil {
		if root := parent.Root(); root != nil {
			switch root.Name {
			case "Scalar":
				prefix = "S_"
			case "Node":
				prefix = "N_"
			case "Word":
				prefix = "W_"
			}
		}
	}
	def := &Type{
		ID:       fmt.Sprintf("%st%011x", prefix, testTypeSeq),
		Name:     parts[len(parts)-1],
		Parent:   parent,
		Origin:   OriginUserDef,
		Behavior: DefaultBehavior,
	}
	if parent != nil {
		def.Rank = parent.Rank
	}
	testTypePool[path] = def
	return def
}

var testTypePool = map[string]*Type{}
var testTypeSeq int

// BuiltinIDForPath returns the canonical Builtin ID for path, or ""
// if the path is not a registered builtin.
func BuiltinIDForPath(path string) string {
	if def := Builtin.bypath[path]; def != nil {
		return def.ID
	}
	return ""
}

// mustBuiltinType returns the Type for a builtin path. Panics if the
// path is not registered — used by the well-known T* constants in
// types.go, where any missing entry is a programmer error.
func mustBuiltinType(path string) *Type {
	def := Builtin.bypath[path]
	if def == nil {
		panic(fmt.Sprintf("typetable: builtin %q not in Builtin table", path))
	}
	return def
}
