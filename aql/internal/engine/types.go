package engine

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Type represents a hierarchical AQL type such as "Scalar/String/Proper" or
// "Scalar/Number/Integer". A child type matches a parent pattern:
// Scalar/String/Proper matches Scalar/String matches Scalar.
//
// Builtin types carry a fixed ID that is stable across runs and independent
// of creation order. The ID format is "<prefix>_" followed by 12 lowercase
// hex characters encoding the type's assigned number. Runtime-created types
// have an empty ID.
type Type struct {
	ID    string
	Parts []string
}

// typeRoots are the top-level type hierarchy roots. If Parts[0] is already
// one of these, the path is considered fully qualified and is not expanded.
var typeRoots = map[string]bool{
	"Scalar": true,
	"Node":   true,
	"Word":   true,
	"Object": true,
	"Any":    true,
	"None":   true,
	"Type":   true,
}

// typeAncestry maps short (legacy) first-part names to their full ancestry
// prefix. NewType auto-expands paths whose first part appears here.
var typeAncestry = map[string]string{
	"String":      "Scalar/String",
	"Number":      "Scalar/Number",
	"Integer":     "Scalar/Number/Integer",
	"Decimal":     "Scalar/Number/Decimal",
	"Boolean":     "Scalar/Boolean",
	"Atom":        "Scalar/Atom",
	"List":        "Node/List",
	"Map":         "Node/Map",
	"Table":       "Object/Table",
	"Record":      "Object/Record",
	"Mark":        "Word/__MK",
	"Move":        "Word/__MV",
	"Forward":     "Word/__FW",
	"Paren":       "Word/__OP",
	"Fndef":       "Word/__FN",
	"Fnundef":     "Word/__UF",
	"Function":    "Word/Function",
	"Returncheck": "Word/__RC",
	"Disjunct":    "Word/__DJ",
	"Module":      "Word/__MD",
	"Resource":    "Object/Resource",
	"Entity":      "Object/Resource/Entity",
	"ScalarType":  "Type/ScalarType",
	"NodeType":    "Type/NodeType",
}

// Well-known types.
var (
	TAny          = mustType("Any")
	TNone         = mustType("None")
	TScalar       = mustType("Scalar")
	TString       = mustType("Scalar/String")
	TStringProper = mustType("Scalar/String/Proper")
	TStringEmpty  = mustType("Scalar/String/Empty")
	TNumber       = mustType("Scalar/Number")
	TInteger      = mustType("Scalar/Number/Integer")
	TDecimal      = mustType("Scalar/Number/Decimal")
	TBoolean      = mustType("Scalar/Boolean")
	TNode         = mustType("Node")
	TList         = mustType("Node/List")
	TListArgs     = mustType("Node/List/Args")
	TMap          = mustType("Node/Map")
	TOptions      = mustType("Node/Map/Options")
	TTable        = mustType("Object/Table")
	TRecord       = mustType("Object/Record")
	TAtom         = mustType("Scalar/Atom")
	TWord         = mustType("Word")
	TFunction     = mustType("Word/Function")
	TForward      = mustType("Word/__FW")
	TOpenParen    = mustType("Word/__OP")
	TParenExpr    = mustType("Word/__PE")
	TFnDef        = mustType("Word/__FN")
	TFnUndef      = mustType("Word/__UF")
	TReturnCheck  = mustType("Word/__RC")
	TDisjunct     = mustType("Word/__DJ")
	TMark         = mustType("Word/__MK")
	TMove         = mustType("Word/__MV")
	TModule       = mustType("Word/__MD")
	TInternal     = mustType("Word/__IN")
	TWordInspect  = mustType("Node/Map/Word/Inspect")
	TTypeInspect  = mustType("Node/Map/Type/Inspect")
	TObject         = mustType("Object")
	TResource       = mustType("Object/Resource")
	TResourceEntity = mustType("Object/Resource/Entity")
	TFetchFunction  = mustType("Object/Fetch")
	TFetchRequest  = mustType("Object/Fetch/Request")
	TFetchResponse = mustType("Object/Fetch/Response")
	TError         = mustType("Node/Error")
	TType          = mustType("Type")
	TScalarType    = mustType("Type/ScalarType")
	TNodeType      = mustType("Type/NodeType")

	// Deprecated aliases — kept temporarily for migration.
	TBooleanTrue    = TBoolean
	TBooleanFalse   = TBoolean
	TWordInspection = TWordInspect
)

// builtinTypeIDs maps fully-qualified builtin type paths to their fixed
// numeric IDs. These assignments are stable: new types are appended at the
// end, existing numbers never change.
var builtinTypeIDs = map[string]int{
	"Any":                      1,
	"None":                     2,
	"Scalar":                   3,
	"Scalar/String":            4,
	"Scalar/String/Proper":     5,
	"Scalar/String/Empty":      6,
	"Scalar/Number":            7,
	"Scalar/Number/Integer":    8,
	"Scalar/Number/Decimal":    9,
	"Scalar/Boolean":           10,
	"Node":                     11,
	"Node/List":                12,
	"Node/List/Args":           13,
	"Node/Map":                 14,
	"Object/Table":             15,
	"Object/Record":            16,
	"Word":                     17,
	"Scalar/Atom":              18,
	"Word/Function":            19,
	"Word/__IN":                20,
	"Word/__FW":                21,
	"Word/__OP":                22,
	"Word/__FN":                23,
	"Word/__UF":                24,
	"Word/__RC":                25,
	"Word/__DJ":                26,
	"Word/__MK":                27,
	"Word/__MV":                28,
	"Word/__MD":                29,
	"Node/Map/Options":         38,
	"Object":                   30,
	"Node/Map/Word/Inspect":    31,
	"Node/Map/Type/Inspect":    32,
	"Object/Fetch":             33,
	"Object/Fetch/Request":     34,
	"Object/Fetch/Response":    35,
	"Object/Resource":          36,
	"Object/Resource/Entity":   37,
	"Type":                     39,
	"Type/ScalarType":          40,
	"Type/NodeType":            41,
}

// formatFixedTypeID formats a fixed numeric ID with the appropriate prefix
// for the given type path, producing the same 14-character format as random
// IDs: "<prefix>_" + 12 hex digits.
func formatFixedTypeID(path string, num int) string {
	prefix := IDPrefixForParts(strings.Split(path, "/"))
	return fmt.Sprintf("%s%012x", prefix, num)
}

// IDPrefixForParts returns the ID prefix for a type given its parts.
func IDPrefixForParts(parts []string) string {
	if len(parts) == 0 {
		return "T_"
	}
	switch parts[0] {
	case "Scalar":
		return "S_"
	case "Node":
		return "N_"
	case "Word":
		return "W_"
	case "Object":
		return "T_"
	case "Type":
		return "T_"
	default:
		return "T_"
	}
}

// mustType is used only for well-known type constants at init time.
// It panics on invalid paths — acceptable because these are compile-time
// constants whose correctness is verified by tests.
// Builtin types receive a fixed ID from the builtinTypeIDs table.
func mustType(path string) Type {
	t, err := NewType(path)
	if err != nil {
		panic(err)
	}
	fullPath := strings.Join(t.Parts, "/")
	if num, ok := builtinTypeIDs[fullPath]; ok {
		t.ID = formatFixedTypeID(fullPath, num)
	}
	return t
}

// NewType creates a Type from a slash-separated path, e.g. "String/Proper".
// Short names are auto-expanded to their full hierarchy path: "String/Proper"
// becomes "Scalar/String/Proper", "Map/Fetch/Request" becomes
// "Object/Fetch/Request", etc.
// Every alphabetic part must begin with an uppercase letter; lowercase is an error.
// Non-letter parts (e.g. numeric literal suffixes like "Number/Integer/42") are allowed.
func NewType(path string) (Type, error) {
	parts := strings.Split(path, "/")
	for _, p := range parts {
		r, _ := utf8.DecodeRuneInString(p)
		if unicode.IsLetter(r) && !unicode.IsUpper(r) {
			return Type{}, fmt.Errorf("aql: type part %q in %q must start with an uppercase letter", p, path)
		}
	}

	// Auto-expand short names to full hierarchy paths.
	// If the first part is already a root, no expansion needed.
	if !typeRoots[parts[0]] {
		if fullPrefix, ok := typeAncestry[parts[0]]; ok {
			expanded := fullPrefix
			if len(parts) > 1 {
				expanded += "/" + strings.Join(parts[1:], "/")
			}
			parts = strings.Split(expanded, "/")
		}
	}

	return Type{Parts: parts}, nil
}

// ResolveTypePath attempts to resolve a name (possibly slash-separated) to a
// known Type. Returns the Type and true if the name is a valid type path that
// is a prefix of (or equal to) a known builtin type. Returns zero Type and
// false otherwise.
func ResolveTypePath(name string) (Type, bool) {
	// Fast path: single-part names handled by the existing typeAncestry or typeRoots.
	if !strings.Contains(name, "/") {
		return Type{}, false
	}

	t, err := NewType(name)
	if err != nil {
		return Type{}, false
	}

	// Validate that the resolved path is a prefix of a known builtin type.
	for _, bt := range builtinTypeList {
		if bt.hasPrefix(t) {
			return t, true
		}
	}
	return Type{}, false
}

// hasPrefix reports whether t starts with the parts of prefix.
func (t Type) hasPrefix(prefix Type) bool {
	if len(t.Parts) < len(prefix.Parts) {
		return false
	}
	for i, p := range prefix.Parts {
		if t.Parts[i] != p {
			return false
		}
	}
	return true
}

// builtinTypeList is the set of all known builtin types for path validation.
var builtinTypeList = []Type{
	TAny, TNone, TScalar, TString, TStringProper, TStringEmpty,
	TNumber, TInteger, TDecimal, TBoolean, TNode, TList, TListArgs,
	TMap, TOptions, TTable, TRecord, TAtom, TWord, TFunction,
	TObject, TResource, TResourceEntity, TType, TScalarType, TNodeType,
}

// Matches reports whether this type satisfies the given pattern.
//   - "Any" pattern matches everything.
//   - A child matches a parent: Scalar/String/Proper matches Scalar/String.
//   - A parent does NOT match a child: Scalar/String does not match Scalar/String/Proper.
func (t Type) Matches(pattern Type) bool {
	// "Any" matches everything unconditionally.
	if len(pattern.Parts) == 1 && pattern.Parts[0] == "Any" {
		return true
	}
	if len(t.Parts) < len(pattern.Parts) {
		return false
	}
	for i, p := range pattern.Parts {
		if t.Parts[i] != p {
			return false
		}
	}
	return true
}

// Specificity returns the depth of the type. More parts = more specific.
func (t Type) Specificity() int {
	return len(t.Parts)
}

// String returns the slash-separated type path.
func (t Type) String() string {
	return strings.Join(t.Parts, "/")
}

// Leaf returns the last part of the type path.
// For example, "Object/Fetch/Request" returns "Request".
func (t Type) Leaf() string {
	if len(t.Parts) == 0 {
		return ""
	}
	return t.Parts[len(t.Parts)-1]
}

// IsSubtypeOf reports whether t is a strict subtype of parent.
// For example: Scalar/String/Proper is a subtype of Scalar/String.
// A type is NOT a subtype of itself.
func (t Type) IsSubtypeOf(parent Type) bool {
	if len(t.Parts) <= len(parent.Parts) {
		return false
	}
	for i, p := range parent.Parts {
		if t.Parts[i] != p {
			return false
		}
	}
	return true
}

// Equal reports whether two types are identical.
func (t Type) Equal(other Type) bool {
	if len(t.Parts) != len(other.Parts) {
		return false
	}
	for i := range t.Parts {
		if t.Parts[i] != other.Parts[i] {
			return false
		}
	}
	return true
}

// builtinTypeParts returns a set of all parts used in well-known types.
// Used to initialize Registry.KnownTypeParts for uniqueness enforcement.
func builtinTypeParts() map[string]bool {
	parts := make(map[string]bool)
	builtins := []Type{
		TAny, TNone, TScalar, TString, TStringProper, TStringEmpty,
		TNumber, TInteger, TDecimal, TBoolean, TNode, TList, TListArgs,
		TMap, TOptions, TTable, TRecord, TAtom, TWord, TFunction, TForward,
		TOpenParen, TParenExpr, TFnDef, TFnUndef, TReturnCheck, TDisjunct, TMark,
		TMove, TModule, TInternal, TWordInspect, TTypeInspect, TObject,
		TResource, TResourceEntity, TFetchFunction, TFetchRequest, TFetchResponse,
		TType, TScalarType, TNodeType,
	}
	for _, t := range builtins {
		for _, p := range t.Parts {
			parts[p] = true
		}
	}
	return parts
}

// MetatypeFor returns the metatype for a given type.
// Scalar subtypes (len>1) → TScalarType, Node subtypes (len>1) → TNodeType,
// everything else (including Scalar and Node themselves) → TType.
func MetatypeFor(t Type) Type {
	if len(t.Parts) > 1 {
		switch t.Parts[0] {
		case "Scalar":
			return TScalarType
		case "Node":
			return TNodeType
		}
	}
	return TType
}

// IsMetaType reports whether t is in the Type/* metatype hierarchy.
func IsMetaType(t Type) bool {
	return len(t.Parts) > 0 && t.Parts[0] == "Type"
}

// ValidateTypeNameParts checks that a type name (slash-separated) does not
// reuse any part that is already known. Returns an error if a conflict is found.
func ValidateTypeNameParts(name string, known map[string]bool) error {
	parts := strings.Split(name, "/")
	for _, p := range parts {
		if known[p] {
			return fmt.Errorf("type: name part %q in %q conflicts with an existing type name", p, name)
		}
	}
	return nil
}
