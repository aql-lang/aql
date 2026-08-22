package help

import "sort"

// Category groups related built-in words so the no-argument `boru describe`
// listing reads as a guided tour rather than one long alphabetical dump, and
// so `boru describe <category>` can show one group at a time.
//
// Membership is data here — a central table rather than a field on each Entry
// — so the whole taxonomy is reviewable in one place (the same approach the
// modules package takes for moduleDocs). The groupings mirror the
// help_<category>.go files. TestCategoryCoverage asserts that every registered
// word belongs to exactly one category and that every listed word is a real
// Entry, so the table cannot silently drift away from the help registry.
type Category struct {
	Name    string   // short token used by `boru describe <category>`
	Summary string   // one-line description shown in the index
	Words   []string // CORE member words — callable with no import
	// Module holds the words this category contributes that live in a
	// loadable module, keyed by module id ("math-util" → its words).
	// They are NOT callable bare: `abs 1` is undefined_word until
	// `import "boru:math-util"` binds MathUtil.abs.
	//
	// Splitting them out is not cosmetic. The table listed 80 of its 249
	// words as though they were builtins; every one of them errors as
	// undefined_word in a bare program, and 44 named no module at all, so
	// `describe` gave a reader no way to discover the import they needed.
	// Rendering them under their module is the whole point of the field.
	Module map[string][]string
}

// ModuleWords returns the category's module-provided words flattened in
// module-id order, for callers that need the full membership (the
// coverage guard) rather than the per-module grouping (the renderer).
func (c Category) ModuleWords() []string {
	ids := make([]string, 0, len(c.Module))
	for id := range c.Module {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []string
	for _, id := range ids {
		out = append(out, c.Module[id]...)
	}
	return out
}

// ModuleIDs returns the category's module ids in display order.
func (c Category) ModuleIDs() []string {
	ids := make([]string, 0, len(c.Module))
	for id := range c.Module {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// categories lists every word category in display order — most fundamental
// first. The order is intentional: it is what the `describe` index walks.
var categories = []Category{
	{"math", "Arithmetic, rounding, and numeric functions.", []string{
		"add", "sub", "mul", "div", "mod", "pow", "with-decimal",
	}, map[string][]string{"math-util": {
		"abs", "negate", "min", "max", "sign", "ceil", "floor", "round", "trunc",
		"round-even", "logb", "scalb", "fma", "sqrt", "cbrt", "exp", "log", "log2",
		"log10", "sin", "cos", "tan", "asin", "acos", "atan", "atan2", "hypot",
		"is-nan", "is-inf", "is-finite", "signbit", "remainder", "copysign",
		"nextafter", "pi", "e",
	}}},
	{"compare", "Comparison and ordering.", []string{
		"lt", "gt", "lte", "gte", "cmp", "tcmp", "eq", "neq", "deq",
	}, nil},
	{"boolean", "Boolean logic and connectives.", []string{
		"and", "or", "not", "xor", "otherwise", "any", "all",
	}, map[string][]string{"logic-util": {
		"nand", "implies", "nor", "iff", "xnor",
	}}},
	{"binary", "Bitwise and shift operators.", nil,
		map[string][]string{"bin-util": {
			"band", "bor", "bxor", "bnot", "bsl", "bsr", "busr",
		}}},
	{"string", "String manipulation. (Substring extraction is the core sequence word `slice`, filed under `list` — no import needed.)", nil,
		map[string][]string{"string-util": {
			"upper", "lower", "concat", "split", "trim", "contains", "indexof",
			"replace", "changecase", "normalize", "repeat", "pad", "match", "escape",
		}}},
	{"stack", "Stack shuffling and inspection.", []string{
		"dup", "swap", "drop", "over", "rot", "nip", "tuck", "dup2", "swap2",
		"drop2", "over2", "depth", "pick", "roll", "stack",
	}, nil},
	{"list", "Lists and sequences: build, slice, and grow.", []string{
		"iota", "range", "reverse", "sort", "flatten", "slice", "take", "shed",
		"append", "push", "pop", "shift", "unshift", "size", "enum", "node",
		"flex",
	}, nil},
	{"xml", "Query and accessor words for Node/Xml elements.", []string{
		"xml-elem", "xml-text", "xml-attr",
	}, nil},
	{"storage", "Variables, value access, references, and lenses.", []string{
		"set", "del", "get", "getr", "dot", "dotr", "has", "context", "keys", "vals",
		"reach", "apply", "rebind", "valof", "referent",
		"patrun", "find", "patterns",
	}, nil},
	{"control", "Control flow, definitions, and functions.", []string{
		"unpack", "unpack-prefix", "codequote", "do", "raise", "if", "case", "for", "while", "break",
		"continue", "def", "undef", "var", "fn", "args",
		"afn", "guard", "error", "force-arity", "usurp", "forward-args",
		"stack-args",
	}, nil},
	{"macro", "Quotation, splicing, and macros.", []string{
		"quote", "unquote", "splice", "word", "macro", "macroexpand", "gensym",
		"canon", "mini", "parse", "emit",
	}, nil},
	{"type", "Types: introspection and construction.", []string{
		"convert", "typeof", "inspect", "make", "refine", "class", "surface",
		"exposes", "gen", "of", "extends", "default", "const",
		"base", "tor", "tand", "tany", "tall", "teq",
		"is", "as", "tis", "istype", "behave", "fnsig", "tnot", "pathof",
	}, map[string][]string{"type-util": {"tpartial"}}},
	{"query", "Query pipelines, iteration, resources, and modules.", []string{
		"join", "unify", "module", "import",
		"export", "each", "for-each", "fold", "scan", "filter", "walk",
		"list", "create", "load", "update", "remove", "between", "outer", "inner",
	}, map[string][]string{"query": {
		"select", "from", "where", "order", "limit", "offset", "distinct",
		"group", "having", "union",
	}}},
	{"concurrent", "Processes, messages, and services.", []string{
		"spawn", "self", "send", "receive", "register", "whereis",
		"unregister", "service", "call", "state-of", "wrap",
	}, nil},
	{"io", "Input and output.", []string{"print"},
		map[string][]string{"io": {
			"printstr", "read", "write", "trace", "stdin", "stdout", "stderr",
		}}},
	{"help", "Help and discovery.", []string{
		"help", "describe",
	}, nil},
}

// Categories returns the word categories in display order. The returned slice
// shares the package table's backing arrays; callers must not mutate it.
func Categories() []Category {
	return categories
}

// LookupCategory returns the category with the given name (case-sensitive),
// and whether one was found.
func LookupCategory(name string) (Category, bool) {
	for _, c := range categories {
		if c.Name == name {
			return c, true
		}
	}
	return Category{}, false
}

// CategoryOf returns the name of the category that contains word, or "" if the
// word is not categorised. Used by the coverage test and by callers that want
// to show a word's group.
func CategoryOf(word string) string {
	for _, c := range categories {
		for _, w := range c.Words {
			if w == word {
				return c.Name
			}
		}
		// Module-provided words are members too — omitting them here
		// would report "" for all 80 of them.
		for _, w := range c.ModuleWords() {
			if w == word {
				return c.Name
			}
		}
	}
	return ""
}
