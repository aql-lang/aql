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
// (capitalised names like Decision.Cond) are not functions and carry no entry.
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
}
