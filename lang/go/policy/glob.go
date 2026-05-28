package policy

import "strings"

// Glob matches s against pat. Supports:
//
//   - exact characters (literal match)
//   - `?`     — matches any single non-separator character
//   - `*`     — matches any run of non-separator characters
//   - `**`   — matches any run including separator characters
//   - `[…]`  — not supported (kept literal — paths and names don't
//     need character classes for this use case)
//
// The separator character is `/` for path-shaped patterns and is
// effectively absent for word-shaped patterns (no `/` ever appears
// in a word name, so `*` and `**` collapse to the same behaviour).
// Callers don't need to differentiate; the matcher does the right
// thing for both.
//
// Empty pattern matches only empty string. Empty string never
// matches a non-empty pattern.
func Glob(pat, s string) bool {
	return globMatch(pat, s)
}

// globMatch is the recursive engine. We walk pat and s in lockstep,
// branching on the meta-tokens. State is purely positional so the
// cost is O(len(pat)*len(s)) worst case — fine for our inputs
// (patterns are tens of chars, names are likewise).
func globMatch(pat, s string) bool {
	for i := 0; i < len(pat); i++ {
		c := pat[i]
		switch c {
		case '*':
			// Detect `**` (double star): consumes separators too.
			doubleStar := i+1 < len(pat) && pat[i+1] == '*'
			if doubleStar {
				// Skip both stars; possibly skip following '/' so
				// `/**/` matches zero or more path components.
				rest := pat[i+2:]
				if strings.HasPrefix(rest, "/") {
					// Try matching with the `/` consumed (zero components)
					// or with the `**/` skipped over zero or more dirs.
					if globMatch(rest[1:], s) {
						return true
					}
					for j := 0; j <= len(s); j++ {
						if j < len(s) && s[j] == '/' {
							if globMatch(rest, s[j:]) {
								return true
							}
						}
					}
					return globMatch(rest, s)
				}
				// Bare `**` at the end of pattern matches everything.
				if rest == "" {
					return true
				}
				// `**` followed by something else: try every suffix of s.
				for j := 0; j <= len(s); j++ {
					if globMatch(rest, s[j:]) {
						return true
					}
				}
				return false
			}
			// Single star: matches any run of non-separator characters.
			rest := pat[i+1:]
			if rest == "" {
				return !strings.ContainsRune(s, '/')
			}
			for j := 0; j <= len(s); j++ {
				if j > 0 && s[j-1] == '/' {
					// Hit a separator; single star can't span it.
					return false
				}
				if globMatch(rest, s[j:]) {
					return true
				}
			}
			return false

		case '?':
			if len(s) == 0 || s[0] == '/' {
				return false
			}
			s = s[1:]

		default:
			if len(s) == 0 || s[0] != c {
				return false
			}
			s = s[1:]
		}
	}
	return len(s) == 0
}

// GlobAny reports whether any pattern in pats matches s. Empty pats
// returns false (no rule matched).
func GlobAny(pats []string, s string) bool {
	for _, p := range pats {
		if Glob(p, s) {
			return true
		}
	}
	return false
}
