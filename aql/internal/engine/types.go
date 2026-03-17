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
type Type struct {
	Parts []string
}

// typeRoots are the top-level type hierarchy roots. If Parts[0] is already
// one of these, the path is considered fully qualified and is not expanded.
var typeRoots = map[string]bool{
	"Scalar": true,
	"Node":   true,
	"Word":   true,
	"Any":    true,
	"None":   true,
}

// typeAncestry maps short (legacy) first-part names to their full ancestry
// prefix. NewType auto-expands paths whose first part appears here.
var typeAncestry = map[string]string{
	"String":      "Scalar/String",
	"Number":      "Scalar/Number",
	"Integer":     "Scalar/Number/Integer",
	"Decimal":     "Scalar/Number/Decimal",
	"Boolean":     "Scalar/Boolean",
	"Atom":        "Word/Atom",
	"List":        "Node/List",
	"Map":         "Node/Map",
	"Table":       "Node/Table",
	"Record":      "Node/Record",
	"Mark":        "Word/Internal/Mark",
	"Move":        "Word/Internal/Move",
	"Forward":     "Word/Internal/Forward",
	"Paren":       "Word/Internal/Paren",
	"Fndef":       "Word/Internal/Fndef",
	"Fnundef":     "Word/Internal/Fnundef",
	"Function":    "Word/Function",
	"Returncheck": "Word/Internal/Return",
	"Disjunct":    "Word/Internal/Disjunct",
	"Module":      "Word/Internal/Module",
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
	TTable        = mustType("Node/Table")
	TRecord       = mustType("Node/Record")
	TAtom         = mustType("Word/Atom")
	TWord         = mustType("Word")
	TFunction     = mustType("Word/Function")
	TForward      = mustType("Word/Internal/Forward")
	TOpenParen    = mustType("Word/Internal/Paren")
	TFnDef        = mustType("Word/Internal/Fndef")
	TFnUndef      = mustType("Word/Internal/Fnundef")
	TReturnCheck  = mustType("Word/Internal/Return")
	TDisjunct     = mustType("Word/Internal/Disjunct")
	TMark         = mustType("Word/Internal/Mark")
	TMove         = mustType("Word/Internal/Move")
	TModule       = mustType("Word/Internal/Module")
	TInternal     = mustType("Word/Internal")
	TWordInspect  = mustType("Node/Map/Word/Inspect")
	TTypeInspect  = mustType("Node/Map/Type/Inspect")
	TFetchFunction = mustType("Word/Function/Fetch")
	TFetchRequest  = mustType("Node/Map/Fetch/Request")
	TFetchResponse = mustType("Node/Map/Fetch/Response")

	// Deprecated aliases — kept temporarily for migration.
	TBooleanTrue    = TBoolean
	TBooleanFalse   = TBoolean
	TWordInspection = TWordInspect
)

// mustType is used only for well-known type constants at init time.
// It panics on invalid paths — acceptable because these are compile-time
// constants whose correctness is verified by tests.
func mustType(path string) Type {
	t, err := NewType(path)
	if err != nil {
		panic(err)
	}
	return t
}

// NewType creates a Type from a slash-separated path, e.g. "String/Proper".
// Short names are auto-expanded to their full hierarchy path: "String/Proper"
// becomes "Scalar/String/Proper", "Map/Fetch/Request" becomes
// "Node/Map/Fetch/Request", etc.
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
// For example, "Node/Map/Fetch/Request" returns "Request".
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
		TMap, TTable, TRecord, TAtom, TWord, TFunction, TForward,
		TOpenParen, TFnDef, TFnUndef, TReturnCheck, TDisjunct, TMark,
		TMove, TModule, TInternal, TWordInspect, TTypeInspect, TFetchFunction,
		TFetchRequest, TFetchResponse,
	}
	for _, t := range builtins {
		for _, p := range t.Parts {
			parts[p] = true
		}
	}
	return parts
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
