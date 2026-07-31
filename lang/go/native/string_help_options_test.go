package native

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native/help"
)

// The help text for a string word lists the options it honours; strOptKeys
// decides which ones it ACCEPTS. Nothing tied the two together, and they
// drifted the moment the key sets became enforced: `u` was a parsed-but-never-
// read field, so it was dropped from every key set, while six help entries
// went on advertising it. `boru describe boru:string-util:split` therefore
// prescribed `{u:true}`, and the runtime answered `string_option_error`.
//
// Documentation that prescribes a rejected input is worse than no
// documentation: the reader has no way to discover it is wrong short of
// running the call. So this test makes the two halves one contract —
// every option named in a word's "Options:" note must be a key the word
// accepts, and every key it accepts must be named.
//
// It deliberately reads the SAME strings a user sees, rather than a
// third list both are checked against: a shared list neither side reads
// would drift from the help text just as easily.

// optionsNote pulls the option names out of a help entry's "Options: …" note.
// The note is prose — entries annotate values ("side (left/right/both)",
// `occ ("first"/"last")`) — so the name is the leading identifier of each
// comma-separated item, and any parenthesised gloss is discarded.
func optionsNote(t *testing.T, word string) ([]string, bool) {
	t.Helper()
	e := help.Lookup(word)
	if e == nil {
		return nil, false
	}
	ident := regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*`)
	for _, n := range e.Notes {
		rest, ok := strings.CutPrefix(n, "Options: ")
		if !ok {
			continue
		}
		rest = strings.TrimSuffix(strings.TrimSpace(rest), ".")
		// Commas inside a parenthesised gloss are not item separators.
		var items []string
		depth, cur := 0, strings.Builder{}
		for _, r := range rest {
			switch {
			case r == '(':
				depth++
			case r == ')':
				depth--
			case r == ',' && depth == 0:
				items = append(items, cur.String())
				cur.Reset()
				continue
			}
			cur.WriteRune(r)
		}
		if strings.TrimSpace(cur.String()) != "" {
			items = append(items, cur.String())
		}
		var names []string
		for _, it := range items {
			if m := ident.FindString(strings.TrimSpace(it)); m != "" {
				names = append(names, m)
			}
		}
		return names, true
	}
	return nil, false
}

func TestStringHelpOptionsMatchAcceptedKeys(t *testing.T) {
	for word, allowed := range strOptKeys {
		documented, ok := optionsNote(t, word)
		if !ok {
			// A word with no "Options:" note documents nothing, which is a
			// gap but not a contradiction — it cannot prescribe a rejected
			// input. Reported so the omission is visible.
			t.Errorf("%s: accepts %v but its help entry has no \"Options:\" note",
				word, sortedKeys(allowed))
			continue
		}
		docSet := map[string]bool{}
		for _, d := range documented {
			docSet[d] = true
			if !allowed[d] {
				t.Errorf("%s: help advertises option %q, which the runtime REJECTS "+
					"(accepted: %s) — a reader following the docs gets string_option_error",
					word, d, strings.Join(sortedKeys(allowed), ", "))
			}
		}
		var undocumented []string
		for k := range allowed {
			if !docSet[k] {
				undocumented = append(undocumented, k)
			}
		}
		sort.Strings(undocumented)
		if len(undocumented) > 0 {
			t.Errorf("%s: accepts %v but its help entry never names them — an option "+
				"nobody can discover is not an option", word, undocumented)
		}
	}
}

// The negative twin: the enumerated-value domains are advertised in the same
// prose, so a value listed in a gloss must be legal. This catches the other
// half of the same drift — renaming a legal value without touching the note.
func TestStringHelpEnumGlossesAreLegalValues(t *testing.T) {
	gloss := regexp.MustCompile(`([A-Za-z][A-Za-z0-9]*)\s*\(([^)]*)\)`)
	for word := range strOptKeys {
		e := help.Lookup(word)
		if e == nil {
			continue
		}
		for _, n := range e.Notes {
			rest, ok := strings.CutPrefix(n, "Options: ")
			if !ok {
				continue
			}
			for _, m := range gloss.FindAllStringSubmatch(rest, -1) {
				key, inner := m[1], m[2]
				legal, isEnum := strOptEnums[key]
				if !isEnum {
					continue // a type hint like "sep (string)", not a value list
				}
				for _, raw := range strings.Split(inner, "/") {
					v := strings.Trim(strings.TrimSpace(raw), `"`)
					if v == "" || !contains(legal, v) {
						continue2 := v == ""
						if continue2 {
							continue
						}
						t.Errorf("%s: help glosses %s as accepting %q, which is not in "+
							"its domain %v", word, key, v, legal)
					}
				}
			}
		}
	}
}
