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
	TAny          = NewType("Any")
	TNone         = NewType("None")
	TScalar       = NewType("Scalar")
	TString       = NewType("String")
	TStringProper = NewType("String/Proper")
	TStringEmpty  = NewType("String/Empty")
	TNumber       = NewType("Number")
	TInteger      = NewType("Number/Integer")
	TBoolean      = NewType("Boolean")
	TBooleanTrue  = NewType("Boolean/True")
	TBooleanFalse = NewType("Boolean/False")
	TAtom         = NewType("Atom")
	TList         = NewType("List")
	TMap          = NewType("Map")
	TWord         = NewType("Word")
	TForward      = NewType("Forward")
	TOpenParen    = NewType("Paren/Open")
	TFnDef        = NewType("Fndef")
	TFnUndef      = NewType("Fnundef")
	TFunction     = NewType("Function")
	TReturnCheck     = NewType("Returncheck")
	TDisjunct        = NewType("Disjunct")
	TWordInspection  = NewType("Map/Word_inspection")
	TMark            = NewType("Mark")
	TMove            = NewType("Move")
	TModule          = NewType("Module")
)

// NewType creates a Type from a slash-separated path, e.g. "String/Proper".
// Every alphabetic part must begin with an uppercase letter; lowercase is an error.
// Non-letter parts (e.g. numeric literal suffixes like "Number/Integer/42") are allowed.
func NewType(path string) Type {
	parts := strings.Split(path, "/")
	for _, p := range parts {
		r, _ := utf8.DecodeRuneInString(p)
		if unicode.IsLetter(r) && !unicode.IsUpper(r) {
			panic(fmt.Sprintf("aql: type part %q in %q must start with an uppercase letter", p, path))
		}
	}
	return Type{Parts: parts}
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
