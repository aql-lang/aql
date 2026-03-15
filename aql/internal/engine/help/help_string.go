package help

func init() {
	register(&Entry{
		Word:    "upper",
		Summary: "Convert a string to uppercase.",
		Signatures: []string{
			"[string] -> [string]",
			"[atom] -> [string]",
		},
		Description: "Converts every character in the input string to its uppercase equivalent " +
			"using Go's strings.ToUpper (Unicode-aware). Works on both string and atom values.",
		Examples: []string{
			`"hello" upper                => 'HELLO'`,
			`"café" upper                 => 'CAFÉ'`,
			`"Hello World" upper          => 'HELLO WORLD'`,
			`"" upper                     => ''`,
		},
		Notes: []string{
			"Also accepts atoms: the word foo upper produces 'FOO'.",
		},
	})

	register(&Entry{
		Word:    "lower",
		Summary: "Convert a string to lowercase.",
		Signatures: []string{
			"[string] -> [string]",
			"[atom] -> [string]",
		},
		Description: "Converts every character in the input string to its lowercase equivalent " +
			"using Go's strings.ToLower (Unicode-aware). Works on both string and atom values.",
		Examples: []string{
			`"HELLO" lower                => 'hello'`,
			`"Café" lower                 => 'café'`,
			`"Hello World" lower          => 'hello world'`,
			`"ABC" lower                  => 'abc'`,
		},
		Notes: []string{
			"Also accepts atoms: the word FOO lower produces 'foo'.",
		},
	})

	register(&Entry{
		Word:    "concat",
		Summary: "Concatenate list elements into a single string.",
		Signatures: []string{
			"[list] -> [string]",
			"[list map] -> [string]",
		},
		Description: "Joins all elements of a list into a single string. Each element is " +
			"converted to its string representation. Use the options map to specify a " +
			"separator (sep) or to skip empty/nullish parts.",
		Examples: []string{
			`["a" "b" "c"] concat                         => 'abc'`,
			`["a" "b" "c"] {sep: ", "} concat              => 'a, b, c'`,
			`["hello" " " "world"] concat                  => 'hello world'`,
			`["a" "" "c"] {skipEmpty: true} concat          => 'ac'`,
			`[1 2 3] {sep: "-"} concat                     => '1-2-3'`,
		},
		Notes: []string{
			"None values are converted to empty strings unless skipNullish is true.",
			"Options: sep (string), skipEmpty (bool), skipNullish (bool).",
		},
	})

	register(&Entry{
		Word:    "split",
		Summary: "Split a string into a list of parts.",
		Signatures: []string{
			"[string string] -> [list]",
			"[string string map] -> [list]",
		},
		Description: "Splits the input string by the separator. By default uses literal matching " +
			"and discards empty parts. Set mode to \"shell\" to use glob-style pattern " +
			"matching as the delimiter.",
		Examples: []string{
			`"a,b,c" "," split                             => ['a','b','c']`,
			`"hello world" " " split                       => ['hello','world']`,
			`"a,,b" "," {keepEmpty: true} split             => ['a','','b']`,
			`"hello" "" split                               => ['h','e','l','l','o']`,
			`" a : b : c " ":" {trimParts: true} split      => ['a','b','c']`,
		},
		Notes: []string{
			"Empty parts are dropped by default; set keepEmpty: true to keep them.",
			"Options: cs, mode, lim, keepEmpty, trimParts, u, norm.",
		},
	})

	register(&Entry{
		Word:    "trim",
		Summary: "Trim whitespace or specific characters from a string.",
		Signatures: []string{
			"[string] -> [string]",
			"[string map] -> [string]",
			"[atom] -> [string]",
			"[atom map] -> [string]",
		},
		Description: "Removes leading and trailing whitespace from the input string. Use the " +
			"options map to trim specific characters (chars) or to trim from only one " +
			"side (side: \"left\" or \"right\").",
		Examples: []string{
			`"  hello  " trim                               => 'hello'`,
			`"xxhelloxx" {chars: "x"} trim                  => 'hello'`,
			`"  hello  " {side: "left"} trim                => 'hello  '`,
			`"  hello  " {side: "right"} trim               => '  hello'`,
		},
		Notes: []string{
			"Default side is \"both\". When chars is set with cs: \"insensitive\", both cases are trimmed.",
			"Options: side (left/right/both), chars, cs, u, norm.",
		},
	})

	register(&Entry{
		Word:    "contains",
		Summary: "Test whether a string contains a search term.",
		Signatures: []string{
			"[string string] -> [boolean]",
			"[string string map] -> [boolean]",
		},
		Description: "Returns true if the input string contains the search string. Supports " +
			"case-insensitive matching, whole-word matching, anchored matching " +
			"(startsWith/endsWith), and shell-pattern matching.",
		Examples: []string{
			`"hello world" "world" contains                           => true`,
			`"hello world" "xyz" contains                             => false`,
			`"Hello" "hello" {cs: "insensitive"} contains             => true`,
			`"hello world" "hello" {anchored: "start"} contains       => true`,
			`"hello world" "world" {anchored: "end"} contains         => true`,
		},
		Notes: []string{
			"anchored: true is equivalent to anchored: \"both\" (exact match).",
			"wholeWord requires matches to be at word boundaries.",
			"Options: cs, mode, anchored, wholeWord, u, norm.",
		},
	})

	register(&Entry{
		Word:    "indexof",
		Summary: "Find the index of a search term in a string.",
		Signatures: []string{
			"[string string] -> [integer]",
			"[string string map] -> [integer]",
		},
		Description: "Returns the byte index of the first (or last) occurrence of the search " +
			"term in the input string. Returns -1 if not found.",
		Examples: []string{
			`"hello world" "world" indexof                            => 6`,
			`"hello world" "xyz" indexof                              => -1`,
			`"abcabc" "abc" {occ: "last"} indexof                    => 3`,
			`"HELLO" "hello" {cs: "insensitive"} indexof             => 0`,
			`"hello" "lo" {from: 4} indexof                          => -1`,
		},
		Notes: []string{
			"Returns byte offsets, not rune offsets.",
			"Options: cs, mode, from, occ (\"first\"/\"last\"), u, norm.",
		},
	})

	register(&Entry{
		Word:    "replace",
		Summary: "Replace occurrences of a search term in a string.",
		Signatures: []string{
			"[string string string] -> [string]",
			"[string string string map] -> [string]",
		},
		Description: "Replaces occurrences of the search string with the replacement string. " +
			"By default replaces only the first occurrence. Use scope: \"all\" to replace all.",
		Examples: []string{
			`"hello world" "world" "earth" replace                    => 'hello earth'`,
			`"aaa" "a" "b" {scope: "all"} replace                    => 'bbb'`,
			`"Hello" "hello" "hi" {cs: "insensitive"} replace        => 'hi'`,
			`"aaa" "a" "b" {scope: "all" count: 2} replace           => 'bba'`,
		},
		Notes: []string{
			"scope defaults to \"first\"; set to \"all\" for global replace.",
			"count limits the maximum number of replacements when scope is \"all\".",
			"Options: cs, mode, scope, from, count, u, norm.",
		},
	})

	register(&Entry{
		Word:    "slice",
		Summary: "Extract a substring by numeric position.",
		Signatures: []string{
			"[string integer] -> [string]",
			"[string integer integer] -> [string]",
			"[string integer map] -> [string]",
			"[string integer integer map] -> [string]",
		},
		Description: "Extracts a substring starting at the given index. If end is omitted, " +
			"slices to the end of the string. Supports negative indices (counted from the end) " +
			"and different counting units (code-unit, code-point, grapheme).",
		Examples: []string{
			`"hello" 0 3 slice                                       => 'hel'`,
			`"hello" 2 slice                                         => 'llo'`,
			`"hello" -3 slice                                        => 'llo'`,
			`"hello" 1 -1 slice                                      => 'ell'`,
		},
		Notes: []string{
			"Default unit is \"code-unit\" (bytes). Use unit: \"code-point\" for rune-based slicing.",
			"Negative indices are Python-style: -1 means one before the end.",
			"Options: unit, fromEnd, u, norm.",
		},
	})

	register(&Entry{
		Word:    "changecase",
		Summary: "Apply a casing transformation to a string.",
		Signatures: []string{
			"[string] -> [string]",
			"[string map] -> [string]",
			"[atom] -> [string]",
			"[atom map] -> [string]",
		},
		Description: "Transforms the case of a string according to the selected style. " +
			"Defaults to \"lower\". Available styles: lower, upper, capitalize (first char), " +
			"title (first char of each word), sentence (first char after lowering all), fold.",
		Examples: []string{
			`"Hello World" changecase                                => 'hello world'`,
			`"hello world" {style: "upper"} changecase               => 'HELLO WORLD'`,
			`"hello world" {style: "title"} changecase               => 'Hello World'`,
			`"hello world" {style: "capitalize"} changecase          => 'Hello world'`,
			`"HELLO WORLD" {style: "sentence"} changecase            => 'Hello world'`,
		},
		Notes: []string{
			"fold is an approximation using toLower; for true Unicode case folding, use a locale-aware library.",
			"Options: style, u, norm, loc.",
		},
	})

	register(&Entry{
		Word:    "normalize",
		Summary: "Normalize Unicode and optionally clean whitespace and line endings.",
		Signatures: []string{
			"[string] -> [string]",
			"[string map] -> [string]",
		},
		Description: "Applies Unicode normalization (default NFC) and optionally trims " +
			"surrounding whitespace, collapses internal whitespace runs, and normalizes " +
			"line endings.",
		Examples: []string{
			`"café" normalize                                        => 'café'`,
			`"  hello  " {trim: true} normalize                      => 'hello'`,
			`"a  b   c" {collapseWs: true} normalize                 => 'a b c'`,
			`"hello" {form: "NFD"} normalize                         => 'hello'`,
		},
		Notes: []string{
			"Whitespace collapsing preserves newlines; only spaces and tabs are collapsed.",
			"Options: form (NFC/NFD/NFKC/NFKD), trim, collapseWs, eol (preserve/lf/crlf).",
		},
	})

	register(&Entry{
		Word:    "repeat",
		Summary: "Repeat a string a fixed number of times.",
		Signatures: []string{
			"[string integer] -> [string]",
			"[string integer map] -> [string]",
		},
		Description: "Returns the input string repeated count times. Optionally insert a " +
			"separator between repetitions using the sep option.",
		Examples: []string{
			`"ab" 3 repeat                                           => 'ababab'`,
			`"ha" 3 {sep: " "} repeat                                => 'ha ha ha'`,
			`"-" 5 repeat                                            => '-----'`,
			`"x" 0 repeat                                            => ''`,
		},
		Notes: []string{
			"Count must be non-negative; negative values produce an error.",
			"Options: sep.",
		},
	})

	register(&Entry{
		Word:    "pad",
		Summary: "Pad a string to a desired length.",
		Signatures: []string{
			"[string integer] -> [string]",
			"[string integer map] -> [string]",
		},
		Description: "Pads the input string to reach the target length. By default pads on " +
			"the right with spaces. Use options to pad left, both sides, or with a " +
			"custom fill string.",
		Examples: []string{
			`"hi" 5 pad                                              => 'hi   '`,
			`"hi" 5 {side: "left"} pad                               => '   hi'`,
			`"hi" 6 {side: "both"} pad                               => '  hi  '`,
			`"hi" 5 {fill: "."} pad                                  => 'hi...'`,
			`"hi" 5 {side: "left" fill: "0"} pad                     => '000hi'`,
			`"hello world" 5 {trunc: true} pad                       => 'hello'`,
		},
		Notes: []string{
			"If the input already meets or exceeds the target length, it is returned unchanged unless trunc is true.",
			"Options: side (left/right/both), fill, trunc.",
		},
	})

	register(&Entry{
		Word:    "match",
		Summary: "Match a pattern and return a structured result.",
		Signatures: []string{
			"[string string] -> [map]",
			"[string string map] -> [map]",
		},
		Description: "Searches for the pattern in the input and returns a map with fields: " +
			"ok (bool), ms (list of match maps), fst (first match), lst (last match), n (count). " +
			"Each match map has m (matched text), i (start index), e (end index).",
		Examples: []string{
			`"hello world" "world" match .ok                         => true`,
			`"hello world" "world" match .fst .m                     => 'world'`,
			`"hello world" "xyz" match .ok                           => false`,
			`"abab" "ab" {scope: "all"} match .n                     => 2`,
			`"hello world" "o" {scope: "all"} match .n               => 2`,
		},
		Notes: []string{
			"Returns a map, not a boolean. Use .ok to get the boolean result.",
			"In shell mode, uses glob matching (* ? [...]).",
			"Options: cs, mode, scope (first/all), u, norm.",
		},
	})

	register(&Entry{
		Word:    "escape",
		Summary: "Escape a string for safe use in shells and text tools.",
		Signatures: []string{
			"[string] -> [string]",
			"[string map] -> [string]",
		},
		Description: "Escapes special characters in the input for the target environment. " +
			"Supports sh, bash, sed, awk, and grep targets. Optionally wraps the result " +
			"in single or double quotes.",
		Examples: []string{
			`"hello world" escape                                    => 'hello\ world'`,
			`"a.b" {tgt: "sed"} escape                               => 'a\.b'`,
			`"a*b" {tgt: "grep"} escape                              => 'a\*b'`,
			`"hello" {quote: "single"} escape                        => ''hello''`,
		},
		Notes: []string{
			"Default target is sh (POSIX shell).",
			"Options: tgt (sh/bash/sed/awk/grep), quote (none/single/double).",
		},
	})
}
