package aqleng

import "fmt"

// ValidateWordName enforces the language-fundamental rule for word
// identifiers:
//
//   - The first character must be in [a-z].
//   - Subsequent characters must be in [a-z], digits [0-9], hyphen
//     `-`, underscore `_`, or trailing `?` (predicate convention).
//
// This is the rule the language design fixes for ALL user-facing
// words: native registrations, `def` bindings, `fn` parameter names,
// and any other path that introduces a name into the def stack.
//
// Engine-internal markers prefixed with `__` are exempt — they're
// plumbing tokens (paren markers, mark/move IDs, return-checks)
// that never appear in user source. The rule is meant to constrain
// what users can name, not what the engine names its own machinery.
//
// Why these characters and not others
// -----------------------------------
//   `[a-z]` first  — uppercase is reserved for type names
//                    (Integer, String, …) so that the engine can
//                    disambiguate type-literal words from value-
//                    word resolution at lookup time. Forcing the
//                    first character to lowercase keeps type-name
//                    words disjoint from user-defined words.
//   `0-9` rest     — common idiom (dup2, swap2, add-two).
//   `-`            — kebab-case is the language's chosen separator
//                    convention (anti-rot, add-two, dup2-alt).
//   `_`            — accepted in mid-name for snake-case
//                    interoperability (fact_acc, double_then_inc),
//                    but NOT as the first character.
//   `?`            — Lisp/Scheme/Ruby predicate convention. Common
//                    enough that production words like `leap-year?`,
//                    `before?`, `equal?` need it. Allowed anywhere
//                    after the first character.
//
// Returns nil for valid names; an *AqlError with code
// "invalid_word_name" otherwise. Callers are expected to surface the
// error in whatever way fits their entry point — Registry methods
// accumulate into r.errs; def/fn handlers return it as a Run-time
// error.
func ValidateWordName(name string) error {
	if name == "" {
		return &AqlError{
			Code:   "invalid_word_name",
			Detail: "word name cannot be empty",
		}
	}
	// Engine-internal marker exemption.
	if len(name) >= 2 && name[0] == '_' && name[1] == '_' {
		return nil
	}
	first := name[0]
	if first < 'a' || first > 'z' {
		return &AqlError{
			Code: "invalid_word_name",
			Detail: fmt.Sprintf(
				"word %q must begin with a lowercase letter [a-z]; got %q",
				name, string(first),
			),
		}
	}
	for i := 1; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '-':
		case c == '_':
		case c == '?':
		default:
			return &AqlError{
				Code: "invalid_word_name",
				Detail: fmt.Sprintf(
					"word %q contains illegal character %q at position %d (allowed: [a-z0-9_-?] after the first letter)",
					name, string(c), i,
				),
			}
		}
	}
	return nil
}
