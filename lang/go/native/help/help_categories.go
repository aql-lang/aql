package help

// Category groups related built-in words so the no-argument `aql describe`
// listing reads as a guided tour rather than one long alphabetical dump, and
// so `aql describe <category>` can show one group at a time.
//
// Membership is data here — a central table rather than a field on each Entry
// — so the whole taxonomy is reviewable in one place (the same approach the
// modules package takes for moduleDocs). The groupings mirror the
// help_<category>.go files. TestCategoryCoverage asserts that every registered
// word belongs to exactly one category and that every listed word is a real
// Entry, so the table cannot silently drift away from the help registry.
type Category struct {
	Name    string   // short token used by `aql describe <category>`
	Summary string   // one-line description shown in the index
	Words   []string // member words, in display order
}

// categories lists every word category in display order — most fundamental
// first. The order is intentional: it is what the `describe` index walks.
var categories = []Category{
	{"math", "Arithmetic, rounding, and numeric functions.", []string{
		"add", "sub", "mul", "div", "mod", "pow", "abs", "negate", "min", "max",
		"sign", "ceil", "floor", "round", "trunc", "round-even", "logb", "scalb",
		"fma", "sqrt", "cbrt", "exp", "log", "log2", "log10", "sin", "cos", "tan",
		"asin", "acos", "atan", "atan2", "hypot", "is-nan", "is-inf", "is-finite",
		"signbit", "remainder", "copysign", "nextafter", "math-pi", "math-e",
		"with-decimal",
	}},
	{"compare", "Comparison and ordering.", []string{
		"lt", "gt", "lte", "gte", "cmp", "tcmp", "eq", "neq", "deq",
	}},
	{"boolean", "Boolean logic and connectives.", []string{
		"and", "or", "not", "xor", "nand", "implies", "nor", "iff", "xnor",
		"otherwise", "any", "all",
	}},
	{"binary", "Bitwise and shift operators.", []string{
		"band", "bor", "bxor", "bnot", "bsl", "bsr", "busr",
	}},
	{"string", "String manipulation.", []string{
		"upper", "lower", "concat", "split", "trim", "contains", "indexof",
		"replace", "changecase", "normalize", "repeat", "pad", "match", "escape",
	}},
	{"stack", "Stack shuffling and inspection.", []string{
		"dup", "swap", "drop", "over", "rot", "nip", "tuck", "dup2", "swap2",
		"drop2", "over2", "depth", "pick", "roll", "stack",
	}},
	{"list", "Lists and sequences: build, slice, and grow.", []string{
		"iota", "range", "reverse", "sort", "flatten", "slice", "take", "shed",
		"append", "push", "pop", "shift", "unshift", "size", "enum", "node",
		"flex",
	}},
	{"storage", "Variables, value access, references, and lenses.", []string{
		"set", "get", "getr", "has", "context", "keys", "vals",
		"reach", "apply", "rebind", "ref", "referent",
	}},
	{"control", "Control flow, definitions, and functions.", []string{
		"unpack", "codequote", "do", "raise", "if", "case", "for", "break",
		"continue", "def", "undef", "var", "fn", "args",
		"afn", "guard", "error", "force-arity", "usurp", "forward-args",
		"stack-args",
	}},
	{"macro", "Quotation, splicing, and macros.", []string{
		"quote", "unquote", "splice", "word", "macro", "macroexpand", "gensym",
		"canon",
	}},
	{"type", "Types: introspection and construction.", []string{
		"convert", "typeof", "inspect", "make", "refine", "class", "surface",
		"exposes", "gen", "of", "extends", "default", "const", "object", "array",
		"base", "tor", "tand", "tany", "tall", "teq", "tpartial",
		"is", "istype", "behave", "fnsig", "tnot", "pathof",
	}},
	{"query", "Query pipelines, iteration, resources, and modules.", []string{
		"select", "from", "where", "order", "limit", "offset", "distinct",
		"group", "having", "join", "union", "unify", "module", "import",
		"export", "each", "for-each", "fold", "scan", "filter",
		"list", "create", "load", "update", "remove", "between", "outer", "inner",
	}},
	{"io", "Input and output.", []string{
		"print", "printstr", "read", "write", "trace", "stdin", "stdout",
		"stderr",
	}},
	{"help", "Help and discovery.", []string{
		"help", "describe",
	}},
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

// CategoryNames returns the category names in display order.
func CategoryNames() []string {
	names := make([]string, len(categories))
	for i, c := range categories {
		names[i] = c.Name
	}
	return names
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
	}
	return ""
}
