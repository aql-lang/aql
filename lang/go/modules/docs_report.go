package modules

func init() {
	registerDocs("boru:report", map[string]string{
		"value":  "Render any value as its single-line print form.",
		"record": "Render a map as a vertical key:value aligned block.",
		"table":  "Render a list of maps as an aligned grid.",
		"list":   "Render a list as indexed one-per-line entries.",
	})

	// Each word renders a DIFFERENT shape; the choice is the whole API and
	// a signature of [Any] cannot express it. Results are from verified
	// lang/spec/module-report.tsv rows.
	registerExamples("boru:report", map[string][]string{
		"value": {`42 Report.value                                  ;# '42' — one value, as text`},
		"record": {
			`{age:30} Report.record                           ;# 'age : 30' — a map as aligned key/value lines`,
			`{} Report.record                                 ;# '(empty record)'`,
		},
		"table": {
			`[{a:1} {a:2}] Report.table                       ;# a column-aligned table of the rows`,
			`[] Report.table                                  ;# '(empty table)'`,
		},
		"list": {
			`[42] Report.list                                 ;# '[0] 42' — index-prefixed lines`,
			`[] Report.list                                   ;# '(empty list)'`,
		},
	})
}
