package eng

import "fmt"

// ValidateWordName enforces the language-fundamental rule for word
// identifiers:
//
//   - The first character must be in [a-z_] (lowercase letter or
//     underscore).
//   - Subsequent characters must be in [a-z0-9_-?] (lowercase
//     letter, digit, hyphen, underscore, or `?`).
//
// This is the rule the language design fixes for ALL user-facing
// words: native registrations, `def` bindings, `fn` parameter names,
// and any other path that introduces a name into the def stack.
//
// Why these characters and not others
// -----------------------------------
//
//	`[a-z_]` first — uppercase is reserved for type names
//	                 (Integer, String, …) so that the engine can
//	                 disambiguate type-literal words from value-
//	                 word resolution at lookup time. Lowercase and
//	                 underscore as starting characters keep user
//	                 words disjoint from type-name fallback words.
//	                 Underscore as a leading character covers the
//	                 discard-placeholder convention (`_`) and the
//	                 engine-internal-marker convention (`__pa`,
//	                 `__mark`, …) under one rule.
//	`0-9` rest     — common idiom (dup2, swap2, add-two).
//	`-`            — kebab-case is the language's chosen separator
//	                 convention (anti-rot, add-two, dup2-alt).
//	`_`            — also accepted mid-name for snake-case
//	                 interoperability (fact_acc, double_then_inc).
//	`?`            — Lisp/Scheme/Ruby predicate convention. Common
//	                 enough that production words like `leap-year?`,
//	                 `before?`, `equal?` need it. Allowed anywhere
//	                 after the first character.
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
	first := name[0]
	if !(first >= 'a' && first <= 'z') && first != '_' {
		return &AqlError{
			Code: "invalid_word_name",
			Detail: fmt.Sprintf(
				"word %q must begin with [a-z_]; got %q",
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
