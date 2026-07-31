package modules

func init() {
	registerDocs("boru:string-util", map[string]string{
		"upper":      "Uppercase a string.",
		"lower":      "Lowercase a string.",
		"concat":     "Join a list of values into one string.",
		"split":      "Split a string on a separator into a list (subject last).",
		"trim":       "Strip leading and trailing whitespace.",
		"contains":   "Report whether a haystack contains a needle (haystack last).",
		"indexof":    "Index of a needle in a haystack (haystack last), -1 if absent.",
		"replace":    "Replace occurrences of a substring (subject last).",
		"changecase": "Recase a string (lower/upper/camel/snake/...).",
		"normalize":  "Unicode-normalise (NFC) and tidy whitespace.",
		"repeat":     "Repeat a string a given number of times (count then string).",
		"pad":        "Pad a string to a width with a fill character.",
		"match":      "Find a pattern in a string, returning the matches (subject last).",
		"escape":     "Escape a string for a target syntax (shell/sed/...).",
	})

	// The words whose behaviour is governed by an OPTIONS MAP, whose keys no
	// signature can show. The plain ones (upper/lower/trim/repeat) keep the
	// generated permutations, which already read correctly.
	// Every line below was run to get its result. The words worth
	// authoring are the ones with an OPTIONS MAP (whose keys no signature
	// can show) or a non-obvious argument order; upper/lower/trim/repeat
	// keep the generated permutations, which already read correctly.
	registerExamples("boru:string-util", map[string][]string{
		"normalize": {
			`StringUtil.normalize 'hello'                     ;# 'hello' — no options, no change`,
			`StringUtil.normalize '  a   b  ' {collapseWs: true trim: true}   ;# 'a b'`,
			`;# Options: form (NFC/NFD/NFKC/NFKD), trim, collapseWs, eol.`,
		},
		"split": {
			`StringUtil.split ',' 'a,b,c'                     ;# ['a' 'b' 'c'] — SEPARATOR first, subject last`,
		},
		"concat": {
			`StringUtil.concat ['a' 'b' 'c']                  ;# 'abc' — it joins a LIST, it is not binary`,
			`StringUtil.concat {sep:'-'} ['a' 'b' 'c']        ;# 'a-b-c'`,
		},
		"match": {
			`StringUtil.match 'ell' 'hello'                   ;# {ok:true ms:[{m:'ell' i:1 e:4}] … n:1}`,
			`(StringUtil.match 'ell' 'hello').ok              ;# true — a MAP, not a Boolean; read .ok`,
			`(StringUtil.match 'l' 'hello' {scope:'all'}).n   ;# 2 — every occurrence`,
			`(StringUtil.match 'h*o' 'hello' {mode:'shell'}).ok   ;# true — glob; the default mode is literal`,
			`;# PATTERN first, subject last. Options: cs, mode (literal/shell), scope (first/all).`,
		},
		"replace": {
			`StringUtil.replace 'l' 'L' 'hello'               ;# 'heLlo' — first only, by default`,
			`StringUtil.replace 'l' 'L' 'hello' {scope:'all'} ;# 'heLLo'`,
		},
		"changecase": {
			`StringUtil.changecase 'hello world' {style:'title'}  ;# 'Hello World'`,
			`StringUtil.changecase 'HELLO' {style:'lower'}    ;# 'hello' — lower is the default`,
			`;# Styles: lower, upper, capitalize, title, sentence, fold.`,
		},
		"pad": {
			`StringUtil.pad 5 'ab'                            ;# 'ab   ' — pad right to a width`,
			`StringUtil.pad 5 {side:'left' fill:'0'} '42'     ;# '00042'`,
		},
		"escape": {
			`StringUtil.escape 'a"b'                          ;# 'a\"b' — for a POSIX shell, by default`,
			`StringUtil.escape "a b" {quote:'single'}         ;# "'a\ b'"`,
			`;# Options: tgt (sh/bash/sed/awk/grep), quote (none/single/double).`,
		},
		"indexof": {
			`StringUtil.indexof 'lo' 'hello'                  ;# 3 — NEEDLE first, -1 when absent`,
		},
		"contains": {
			`StringUtil.contains 'ell' 'hello'                ;# true — needle first`,
		},
	})
}
