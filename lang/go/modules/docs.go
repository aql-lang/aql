package modules

// moduleDocs maps a native module's import id (its desc.Ref, e.g.
// "aql:array-util") to a per-export one-line summary keyed by the EXPORT name
// — the dotted suffix a user types, e.g. "indices" for ArrayUtil.indices.
//
// stampExportProvenance (modules.go) applies these as FnDefInfo.Doc so
// `describe Namespace.word` shows a real summary alongside the signatures and
// module provenance. Keeping the docs here as data — rather than threading a
// doc string through each module's bespoke FnDef maker — gives one uniform
// place to author and review them. TestModuleExportDocs (docs_test.go)
// asserts every function export has an entry, so this table cannot silently
// fall behind the export set.
//
// The table is assembled from per-module files (docs_<name>.go) via
// registerDocs in their init(), so each module's docs live in their own file
// and can be authored independently. Each summary is a single line: a terse
// statement of what the word does, matching the verified
// lang/spec/module-*.tsv row descriptions where one exists. Type exports
// (capitalised names) are not functions and carry no entry.
var moduleDocs = map[string]map[string]string{}

// registerDocs records the per-export doc summaries for one module, keyed by
// its import id (e.g. "aql:array-util"). Called from per-module docs_*.go
// init() functions. Entries merge into any existing map for the id so the
// docs for one module may be split across files if ever convenient.
func registerDocs(ref string, docs map[string]string) {
	m := moduleDocs[ref]
	if m == nil {
		m = make(map[string]string, len(docs))
		moduleDocs[ref] = m
	}
	for k, v := range docs {
		m[k] = v
	}
}

// moduleExamples is the docs table's twin for `describe` EXAMPLES: import
// id → export name → the lines shown verbatim under "Examples:".
//
// It exists because the fallback is worse than nothing for a capability
// word. With no authored examples, describe generates positional
// permutations from the signature — for `Net.fetch` that yields
// `Net.fetch 'a' {a:1,b:2}`, which is well-formed, runs nowhere, and
// hides the only thing a reader needs (what goes IN the options map).
// A summary line cannot carry that; a worked call can.
//
// Unlike moduleDocs this is OPT-IN: an export with no entry keeps the
// generated permutations, which are genuinely useful for the pure
// helpers (`ArrayUtil.reshape 2 3 [1 2 3 4 5 6]` reads fine). The
// ratchet is the other direction — TestModuleExampleKeys asserts every
// key names a real export, so the table cannot rot as words are renamed.
//
// Authoring rules, so the lines stay trustworthy:
//   - Write what a user would actually type, including the `import` when
//     the example needs one to make sense on its own.
//   - Comment with `;# …` in the same style as the core words' examples.
//   - Never show a result that is not what the word returns.
var moduleExamples = map[string]map[string][]string{}

// registerExamples records per-export `describe` examples for one module,
// keyed by its import id. Called from per-module docs_*.go init()
// functions, next to that module's registerDocs call.
func registerExamples(ref string, examples map[string][]string) {
	m := moduleExamples[ref]
	if m == nil {
		m = make(map[string][]string, len(examples))
		moduleExamples[ref] = m
	}
	for k, v := range examples {
		m[k] = v
	}
}

func init() {
	registerDocs("aql:array-util", map[string]string{
		"shape":     "Shape: the length along each axis of a nested list.",
		"rank":      "Rank: the number of axes (nesting depth) of a list.",
		"reshape":   "Lay flat data into the given shape.",
		"transpose": "Swap rows and columns of a rank-2 list.",
		"where":     "Indices of the truthy elements.",
		"grade":     "Indices that would sort the list ascending.",
		"at":        "Select data elements by an index list.",
		"insert-at": "New list with an element inserted at an index (insert-at idx elem list).",
		"remove-at": "New list with the element at an index removed (remove-at idx list).",
		"sortby":    "Reorder a list by a parallel list of sort keys.",
		"replicate": "Repeat each element by its count.",
		"expand":    "Spread data into true slots, 0 into false slots.",
		"compress":  "Keep data elements where the mask is true.",
		"eachrank":  "Apply a body to each rank-N cell of a nested list.",
		"foldaxis":  "Reduce a body along the given axis.",
		"member":    "Boolean mask: is each needle present in the haystack.",
		"unique":    "Distinct elements, in first-seen order.",
		"indices":   "Position of each needle in the haystack, -1 if absent.",
		"group":     "Group values by key (or by value) into a map of buckets.",
		"window":    "All contiguous sub-slices of the given width.",
		"pairs":     "Consecutive overlapping 2-tuples.",
	})

	// Every result below is copied from a verified lang/spec/module-array.tsv
	// row. The words worth authoring here are the ones whose meaning is in
	// the SHAPE of the operands, which a signature of [List List] cannot
	// show; the arithmetic-flavoured ones keep the generated permutations.
	registerExamples("aql:array-util", map[string][]string{
		"shape":     {`ArrayUtil.shape [[1 2 3] [4 5 6]]                ;# [2 3] — length along each axis`},
		"rank":      {`ArrayUtil.rank [[1 2 3] [4 5 6]]                 ;# 2 — number of axes`},
		"reshape":   {`ArrayUtil.reshape [2 3] [1 2 3 4 5 6]            ;# [[1 2 3] [4 5 6]]`},
		"transpose": {`ArrayUtil.transpose [[1 2 3] [4 5 6]]            ;# [[1 4] [2 5] [3 6]]`},
		"where":     {`ArrayUtil.where [true false true false true]     ;# [0 2 4] — indices of the truthy slots`},
		"grade": {
			`ArrayUtil.grade [30 10 20]                       ;# [1 2 0] — the indices that WOULD sort it`,
			`ArrayUtil.at (ArrayUtil.grade [30 10 20]) [30 10 20]   ;# [10 20 30] — apply the grading`,
		},
		"at":      {`ArrayUtil.at [0 2] [10 20 30 40]                  ;# [10 30] — select by index list`},
		"member":  {`ArrayUtil.member [1 5 3] [1 2 3]                  ;# [true false true] — mask, one per needle`},
		"indices": {`ArrayUtil.indices [20 99 10] [10 20 30]           ;# [1 -1 0] — position, -1 when absent`},
		"unique":  {`ArrayUtil.unique [1 2 2 3 3 3]                    ;# [1 2 3] — first-seen order`},
		"group": {
			`ArrayUtil.group [1 2 3]                          ;# {1:[0] 2:[1] 3:[2]} — by VALUE, to index`,
			`ArrayUtil.group ['a' 'b' 'a'] [1 2 3]            ;# {'a':[1 3] 'b':[2]} — by parallel key list`,
		},
		"window":    {`ArrayUtil.window 2 [1 2 3 4]                     ;# [[1 2] [2 3] [3 4]] — overlapping slices`},
		"pairs":     {`ArrayUtil.pairs [1 2 3 4]                        ;# [[1 2] [2 3] [3 4]] — window 2`},
		"compress":  {`ArrayUtil.compress [true false true] [1 2 3]     ;# [1 3] — keep where the mask is true`},
		"expand":    {`ArrayUtil.expand [true false true] [7 8]         ;# [7 0 8] — spread into the true slots`},
		"replicate": {`ArrayUtil.replicate [2 0 3] [1 2 3]              ;# [1 1 3 3 3] — repeat each by its count`},
		"sortby":    {`ArrayUtil.sortby [3 1 2] ['c' 'a' 'b']           ;# ['a' 'b' 'c'] — reorder by parallel keys`},
		"insert-at": {
			`ArrayUtil.insert-at 1 99 [1 2 3]                 ;# [1 99 2 3] — a NEW list; the input is untouched`,
			`ArrayUtil.insert-at 3 99 [1 2 3]                 ;# [1 2 3 99] — index len appends`,
			`ArrayUtil.insert-at 4 99 [1 2 3]                 ;# raises index_out_of_range`,
		},
		"remove-at": {`ArrayUtil.remove-at 1 [1 2 3]                    ;# [1 3] — a NEW list`},
		"eachrank": {
			`ArrayUtil.eachrank 0 [mul 2] [[1 2] [3 4]]       ;# [[2 4] [6 8]] — body per rank-0 cell (each leaf)`,
			`;# The BODY is the middle operand, not the last: rank, body, data.`,
		},
		"foldaxis": {
			`ArrayUtil.foldaxis 0 [add] [[1 2] [3 4]]         ;# [4 6] — reduce down each column`,
			`ArrayUtil.foldaxis 1 [add] [[1 2] [3 4]]         ;# [3 7] — reduce along each row`,
		},
	})
}
