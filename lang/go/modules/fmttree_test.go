package modules

import (
	"strconv"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/formatter"
	"github.com/aql-lang/aql/lang/go/native"
)

// TestFmtTreeStructure drives Fmt.tree over source exercising every node kind
// and checks the bridge shape: each node is a `$kind`-tagged Map carrying
// `text` and `children`, and every NodeKind maps to its stable dispatch name.
// The rich source hits all 16 NodeKindName arms.
func TestFmtTreeStructure(t *testing.T) {
	src := "def x \"hi\"  # c\n[1, 2]\n{a:1}\n(f a)\nm . k\nx?\n!y\na|b\na; b\n"
	out, err := fmtTreeHandler([]native.Value{native.NewString(src)}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Fmt.tree: %v", err)
	}
	root, merr := native.AsMap(out[0])
	if merr != nil {
		t.Fatalf("Fmt.tree result not a map: %v", merr)
	}
	// The root node reports kind `root` and carries the three uniform keys.
	if k, _ := root.Get("$kind"); func() bool { a, _ := k.AsConcreteAtom(); return a != "root" }() {
		t.Errorf("root $kind is not 'root'")
	}
	for _, key := range []string{"$kind", "text", "children"} {
		if _, ok := root.Get(key); !ok {
			t.Errorf("node missing uniform key %q", key)
		}
	}

	// Collect every $kind reachable in the bridged tree.
	seen := map[string]bool{}
	var walk func(native.Value)
	walk = func(v native.Value) {
		m, e := native.AsMap(v)
		if e != nil {
			return
		}
		if kv, ok := m.Get("$kind"); ok {
			if a, ae := kv.AsConcreteAtom(); ae == nil {
				seen[a] = true
			}
		}
		if cv, ok := m.Get("children"); ok {
			if lst, le := native.AsList(cv); le == nil {
				for i := 0; i < lst.Len(); i++ {
					walk(lst.Get(i))
				}
			}
		}
	}
	walk(out[0])

	want := []string{
		"root", "word", "string", "number", "comment", "list", "map",
		"paren", "comma", "colon", "semicolon", "dot", "question", "bang",
		"pipe", "newline",
	}
	for _, k := range want {
		if !seen[k] {
			t.Errorf("bridged tree missing node kind %q", k)
		}
	}
}

// TestFmtTreeRejectsNonConcrete pins the panic-prevention guard: a type
// literal in the source slot is refused with an error, never dereferenced.
func TestFmtTreeRejectsNonConcrete(t *testing.T) {
	if _, err := fmtTreeHandler([]native.Value{native.NewTypeLiteral(native.TString)}, nil, nil, nil); err == nil {
		t.Fatal("Fmt.tree accepted a non-concrete (type-literal) argument; want error")
	}
}

// TestNodeKindNameUnknown covers the closed-enum fallthrough: an out-of-range
// NodeKind (never built by the tokeniser) names as "unknown".
func TestNodeKindNameUnknown(t *testing.T) {
	if got := formatter.NodeKindName(formatter.NodeKind(9999)); got != "unknown" {
		t.Errorf("NodeKindName(out-of-range) = %q, want unknown", got)
	}
}

// TestFmtTreeDeclarativeFormatter is the Phase-3 payoff: a formatter for AQL
// SOURCE written as a declarative AQL rule table over Fmt.tree, dispatching by
// Fmt.kind and recursing through `node.children`. For attachment-free
// statements (no `:`/`.`/`,` gluing) inline rendering is a plain space-join,
// so the declarative output equals the built-in Fmt.format byte-for-byte —
// proving the vocabulary reaches and formats real source, not just data.
func TestFmtTreeDeclarativeFormatter(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	reg.SetParseFunc(parser.Parse)
	InstallResolver(reg)

	// The declarative source formatter: rules keyed by node kind, an `inline`
	// helper that space-joins a node's formatted children, and `apply` that
	// dispatches. Handles leaves and the bracket containers — the
	// attachment-free core.
	const rules = `import "aql:fmt"
def apply fn [nd:Any Any [nd (rules get (Fmt.kind nd))]]
def inline fn [nd:Any Any [join " " ((nd.children) each [apply])]]
def rules {
  root:   (nd:Any => (inline nd))
  word:   (nd:Any => (nd.text))
  string: (nd:Any => (nd.text))
  number: (nd:Any => (nd.text))
  list:   (nd:Any => (join "" ["[" (inline nd) "]"]))
  paren:  (nd:Any => (join "" ["(" (inline nd) ")"]))
}
`

	// Each case is an attachment-free single statement; the declarative
	// formatter must reproduce Fmt.format exactly.
	cases := []string{
		"def x 1",
		"[1 2 3]",
		"(add 1 2)",
		"def xs [1 2 3]",
		"[1 [2 3] 4]",
		"(f (g 1) 2)",
	}
	for _, src := range cases {
		prog := rules + "apply (Fmt.tree " + strconv.Quote(src) + ")"
		values, perr := parser.Parse(prog)
		if perr != nil {
			t.Fatalf("parse %q: %v", src, perr)
		}
		stack, rerr := native.NewTop(reg).Run(values)
		if rerr != nil {
			t.Fatalf("run %q: %v", src, rerr)
		}
		if len(stack) != 1 {
			t.Fatalf("%q: stack depth %d, want 1", src, len(stack))
		}
		got, cerr := stack[0].AsConcreteString()
		if cerr != nil {
			t.Fatalf("%q: result not a concrete string: %v", src, cerr)
		}
		// The built-in emitter is the oracle. Attachment-free statements fit
		// on one line, so Format has no trailing-newline-only differences
		// beyond the final "\n" the declarative single-node render omits.
		want := formatter.Format(src)
		if want[len(want)-1] == '\n' {
			want = want[:len(want)-1]
		}
		if got != want {
			t.Errorf("declarative source format of %q = %q, want %q", src, got, want)
		}
	}
}
