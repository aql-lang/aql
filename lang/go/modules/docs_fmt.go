package modules

func init() {
	registerDocs("boru:fmt", map[string]string{
		"format":          "Pretty-print boru source text into canonical layout.",
		"format-markdown": "Reformat boru in ```boru fences and <!-- borufmt --> regions of Markdown.",
		"format-html":     "Reformat boru in <!-- borufmt --> regions of an HTML document.",
		"render":          "Lay out a declarative document tree (text/line/group/indent) to a width.",
		"kind":            "Classify a node for rule dispatch: a $kind-tagged Map's tag, else 'map' / 'list' / 'scalar'.",
		"children":        "The child sequence a rule recurses over: a Map's {$kind:'entry' key value} entries, a List's elements, else [].",
		"tree":            "Parse boru source into its layout CST as a $kind-tagged value tree, so a formatter can be written as declarative boru rules.",
		"rules":           "The canonical layout rule table (width/indent/attach/templates/strategies) — defined in boru (formatter/fmt-rules.boru), the stylesheet Fmt.format interprets.",
		"format-with":     "Format boru source under a (partial) rule table: override width, indent, attach classes, per-kind templates, statement strategies.",
	})

	// Fmt is the pretty-printer behind `boru fmt`. Its two halves look
	// alike in the signatures and are not: format* REFORMATS boru source,
	// while render/tree/children work on a document tree of layout nodes.
	// Results are from verified lang/spec/module-fmt.tsv rows.
	registerExamples("boru:fmt", map[string][]string{
		"format": {
			`Fmt.format "[ a  b  c ]"                         ;# '[a b c]\n'`,
			`Fmt.format "def double fn [[x:Integer] [Integer] [x mul 2]]"`,
			`;# 'def double fn x:Integer Integer [x mul 2]\n' — canonical form`,
		},
		"format-markdown": {
			"Fmt.format-markdown \"```boru\\ndef  x  1\\n```\"       ;# reformats only the boru fences",
		},
		"format-html": {
			`Fmt.format-html "<!-- borufmt -->\ndef  z   3\n<!-- /borufmt -->"`,
			`;# reformats only what is between the borufmt markers`,
		},
		"format-with": {`Fmt.format-with {width: 60} "def x 1"            ;# same, with explicit layout rules`},
		"kind":        {`{a:1} Fmt.kind                                   ;# map/q — the layout kind of a value`},
		"render": {
			`Fmt.render 40 {fmt:"group" body:{fmt:"concat" parts:["a" {fmt:"line"} "b"]}}   ;# 'a b'`,
			`Fmt.render 1  {fmt:"group" body:{fmt:"concat" parts:["a" {fmt:"line"} "b"]}}   ;# 'a\nb'`,
			`;# The first operand is the WIDTH: a group breaks only when it`,
			`;# does not fit, which is what makes the output width-aware.`,
		},
		"tree":     {`Fmt.tree "def x 1"                               ;# the layout document, as data`},
		"children": {`Fmt.children (Fmt.tree "def x 1")                ;# walk a layout node's children`},
		"rules":    {`Fmt.rules                                        ;# the active layout rules`},
	})
}
