package engine

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Type represents a hierarchical AQL type such as "string/proper" or "number/integer".
// A child type matches a parent pattern: string/proper matches string.
type Type struct {
	Parts []string
}

// Well-known types.
var (
	TAny          = mustType("Any")
	TNone         = mustType("None")
	TScalar       = mustType("Scalar")
	TString       = mustType("String")
	TStringProper = mustType("String/Proper")
	TStringEmpty  = mustType("String/Empty")
	TNumber       = mustType("Number")
	TInteger      = mustType("Number/Integer")
	TBoolean      = mustType("Boolean")
	TBooleanTrue  = mustType("Boolean/True")
	TBooleanFalse = mustType("Boolean/False")
	TAtom         = mustType("Atom")
	TList         = mustType("List")
	TMap          = mustType("Map")
	TWord         = mustType("Word")
	TForward      = mustType("Forward")
	TOpenParen    = mustType("Paren/Open")
	TFnDef        = mustType("Fndef")
	TFnUndef      = mustType("Fnundef")
	TFunction     = mustType("Function")
	TReturnCheck     = mustType("Returncheck")
	TDisjunct        = mustType("Disjunct")
	TWordInspection  = mustType("Map/Word_inspection")
	TMark            = mustType("Mark")
	TMove            = mustType("Move")
	TModule          = mustType("Module")
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
	return Type{Parts: parts}, nil
}

// Matches reports whether this type satisfies the given pattern.
//   - "any" pattern matches everything.
//   - A child matches a parent: string/proper matches string.
//   - A parent does NOT match a child: string does not match string/proper.
func (t Type) Matches(pattern Type) bool {
	if len(pattern.Parts) == 1 && pattern.Parts[0] == "Any" {
		// "Any" matches all data types but not internal types (Word, Forward).
		if t.Parts[0] == "Word" || t.Parts[0] == "Forward" || t.Parts[0] == "Paren" || t.Parts[0] == "Mark" || t.Parts[0] == "Move" || t.Parts[0] == "Returncheck" {
			return false
		}
		return true
	}
	if len(pattern.Parts) == 1 && pattern.Parts[0] == "Scalar" {
		// "Scalar" is the supertype of String, Number, Boolean, and Atom.
		switch t.Parts[0] {
		case "String", "Number", "Boolean", "Atom", "Scalar":
			return true
		}
		return false
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

// IsSubtypeOf reports whether t is a strict subtype of parent.
// For example: string/proper is a subtype of string, number/integer is a subtype of number.
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
