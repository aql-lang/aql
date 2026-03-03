package engine

import "strings"

// Type represents a hierarchical AQL type such as "string/proper" or "number/integer".
// A child type matches a parent pattern: string/proper matches string.
type Type struct {
	Parts []string
}

// Well-known types.
var (
	TAny         = NewType("any")
	TNone        = NewType("none")
	TString      = NewType("string")
	TStringProper = NewType("string/proper")
	TStringEmpty = NewType("string/empty")
	TNumber      = NewType("number")
	TInteger     = NewType("number/integer")
	TBoolean     = NewType("boolean")
	TBooleanTrue = NewType("boolean/true")
	TBooleanFalse = NewType("boolean/false")
	TList        = NewType("list")
	TMap         = NewType("map")
	TWord        = NewType("word")
	TForward     = NewType("forward")
	TOpenParen   = NewType("paren/open")
	TFnDef       = NewType("fndef")
)

// NewType creates a Type from a slash-separated path, e.g. "string/proper".
func NewType(path string) Type {
	return Type{Parts: strings.Split(path, "/")}
}

// Matches reports whether this type satisfies the given pattern.
//   - "any" pattern matches everything.
//   - A child matches a parent: string/proper matches string.
//   - A parent does NOT match a child: string does not match string/proper.
func (t Type) Matches(pattern Type) bool {
	if len(pattern.Parts) == 1 && pattern.Parts[0] == "any" {
		// "any" matches all data types but not internal types (word, forward).
		if t.Parts[0] == "word" || t.Parts[0] == "forward" || t.Parts[0] == "paren" || t.Parts[0] == "fndef" {
			return false
		}
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
