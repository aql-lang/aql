package modules

func init() {
	registerDocs("boru:sift", map[string]string{
		"define":   "Register a spec-map as a named parse kind: define NAME SPEC.",
		"parse":    "Parse a source with an inline spec-map or a registered kind name.",
		"kinds":    "List the atoms of every sift-registered kind (families, presets, user defines).",
		"families": "List the six format-family atoms: kv, blocks, columns, dsv, fixed, pattern.",
		"spec":     "The registered spec-map for a kind name (introspection): spec NAME.",
		"detect":   "The kind a path or argv resolves to via the detection table, or None.",
		"check":    "Validate a spec-map without registering; a list of {code detail} issues, empty when valid.",
	})

	// Sift parses semi-structured text by FAMILY plus an options map, and
	// the family atom is the whole API — a signature of [Atom Map String]
	// names it without saying what may go there. Results are from verified
	// lang/spec/module-sift.tsv rows.
	registerExamples("boru:sift", map[string][]string{
		"families": {`Sift.families                                    ;# [kv/q blocks/q columns/q dsv/q fixed/q pattern/q]`},
		"kinds":    {`Sift.kinds                                       ;# the families plus any Sift.define'd names`},
		"parse": {
			`Sift.parse kv/q {sep:':'} "a: 1\nb: 2"           ;# {a:'1' b:'2'} — values are Strings by default`,
			`(Sift.parse kv/q {sep:':' vtype:'integer'} "a: 1\nb: 2") dot a   ;# 1 — typed instead`,
			`(Sift.parse kv/q {sep:':' names:'normalize'} "MemTotal: 4") dot mem-total   ;# '4'`,
			`Sift.parse kv/q {sep:':' repeat:'list'} "a: 1\na: 2"   ;# {a:['1' '2']} — repeat: first/last/list`,
			`Sift.parse kv/q {sep:':' strict:'skip'} "bad\na: 1"    ;# {a:'1'} — skip unparseable lines`,
			`size (Sift.parse dsv/q {sep:':' fields:['u' 'p']} "root:x\ndaemon:y")   ;# 2`,
			`Sift.parse {family:'kv' opts:{sep:'='}} "a=1"     ;# {a:'1'} — a spec map instead of family+opts`,
		},
		"define": {
			`Sift.define ex {family:'kv' opts:{sep:'='}}`,
			`Sift.parse ex/q "a=1"                            ;# {a:'1'} — now a first-class kind`,
		},
		"spec": {
			`Sift.define ex {family:'kv'}`,
			`(Sift.spec ex) dot family                        ;# kv/q — read a defined kind back`,
		},
		"detect": {
			`Sift.define m {family:'kv' detect:{path:["/proc/meminfo"]}}`,
			`Sift.detect "/proc/meminfo"                      ;# m/q — pick the kind by path or command`,
			`Sift.detect "/nope"                              ;# none`,
		},
		"check": {
			`Sift.check {family:'kv'}                         ;# [] — no problems`,
			`(Sift.check {family:'bogus'}) dot 0 dot code     ;# sift_spec_error/q — validate BEFORE parsing`,
		},
	})
}
