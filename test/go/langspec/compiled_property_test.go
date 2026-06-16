// Property-based differential gate: instead of the finite curated corpus,
// GENERATE random well-typed AQL programs from the compilable subset and assert
// the compiled VM and the interpreter agree on every one (the same invariant
// TestSpecCompiledDifferential checks per spec row). The corpus exercises hand-
// written rows; this exercises COMBINATIONS the corpus never enumerates (a
// computed list whose elements are `if` results, `size` over an assembled list,
// a def-local feeding arithmetic, …) — exactly where the carrier compiler's
// per-construct gates can have holes. A failure is shrunk to a minimal program.
package langspec

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
)

// fuzzEnv reads a positive integer from an env var, else returns def. Lets CI /
// a nightly job CRANK the fuzzer (`AQL_FUZZ_SEEDS=20 AQL_FUZZ_ITERS=5000`)
// without touching the lean, deterministic default the standard suite runs.
func fuzzEnv(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// cat is a generated node's value category, so generation and shrinking stay
// well-typed (an Integer slot never gets a Boolean node).
type cat int

const (
	cInt cat = iota
	cBool
	cList
	cMap
)

type gnode struct {
	op   string
	cat  cat
	n    int      // lit value / var index / map-key index
	cmp  string   // for op=="cmp"
	keys []string // for op=="map"
	kids []*gnode
}

var mapKeys = []string{"a", "b", "c"}

var cmps = []string{"lt", "gt", "eq", "lte", "gte"}

// gen builds a node of the requested category within a depth budget; scope is
// the in-scope Integer var names (def-bound).
func gen(r *rand.Rand, c cat, depth int, scope []string) *gnode {
	switch c {
	case cBool:
		return &gnode{op: "cmp", cat: cBool, cmp: cmps[r.Intn(len(cmps))],
			kids: []*gnode{gen(r, cInt, depth-1, scope), gen(r, cInt, depth-1, scope)}}
	case cMap:
		nk := 1 + r.Intn(len(mapKeys))
		m := &gnode{op: "map", cat: cMap, keys: append([]string{}, mapKeys[:nk]...)}
		for i := 0; i < nk; i++ {
			m.kids = append(m.kids, gen(r, cInt, depth-1, scope))
		}
		return m
	case cList:
		if depth > 0 && r.Intn(3) == 0 {
			// `for <count> [<body using i>]` -> list of per-iteration values.
			return &gnode{op: "for", cat: cList, kids: []*gnode{
				gen(r, cInt, depth-1, scope), gen(r, cInt, depth-1, append(append([]string{}, scope...), "i"))}}
		}
		k := 1 + r.Intn(3)
		l := &gnode{op: "list", cat: cList}
		for i := 0; i < k; i++ {
			l.kids = append(l.kids, gen(r, cInt, depth-1, scope))
		}
		return l
	default: // cInt
		if depth <= 0 {
			if len(scope) > 0 && r.Intn(2) == 0 {
				return &gnode{op: "var", cat: cInt, n: r.Intn(len(scope))}
			}
			return &gnode{op: "lit", cat: cInt, n: r.Intn(6)}
		}
		switch r.Intn(10) {
		case 0, 1:
			ops := []string{"add", "sub", "mul"}
			return &gnode{op: ops[r.Intn(len(ops))], cat: cInt,
				kids: []*gnode{gen(r, cInt, depth-1, scope), gen(r, cInt, depth-1, scope)}}
		case 2:
			return &gnode{op: "if", cat: cInt, kids: []*gnode{
				gen(r, cBool, depth-1, scope), gen(r, cInt, depth-1, scope), gen(r, cInt, depth-1, scope)}}
		case 3:
			return &gnode{op: "size", cat: cInt, kids: []*gnode{gen(r, cList, depth-1, scope)}}
		case 4:
			// `(<map> get <key>)` — key chosen from the map's own keys.
			m := gen(r, cMap, depth-1, scope)
			return &gnode{op: "get", cat: cInt, n: r.Intn(len(m.keys)), kids: []*gnode{m}}
		default:
			if len(scope) > 0 && r.Intn(3) == 0 {
				return &gnode{op: "var", cat: cInt, n: r.Intn(len(scope))}
			}
			return &gnode{op: "lit", cat: cInt, n: r.Intn(6)}
		}
	}
}

// genProgram returns a top-level node and the var names it may reference. With
// some probability it wraps the body in one or two def-bindings to stress
// value-def locals (OpStoreLocal) and local references.
func genProgram(r *rand.Rand, depth int) (*gnode, []string) {
	scope := []string{}
	var defs []*gnode
	for r.Intn(3) == 0 && len(scope) < 2 {
		name := fmt.Sprintf("v%d", len(scope))
		defs = append(defs, &gnode{op: "def", n: len(scope), kids: []*gnode{gen(r, cInt, depth-1, append([]string{}, scope...))}})
		scope = append(scope, name)
	}
	bodyCat := cInt
	switch r.Intn(5) {
	case 0:
		bodyCat = cList
	case 1:
		bodyCat = cMap
	}
	body := gen(r, bodyCat, depth, scope)
	if len(defs) == 0 {
		return body, scope
	}
	return &gnode{op: "seq", kids: append(defs, body)}, scope
}

func render(n *gnode, scope []string) string {
	switch n.op {
	case "lit":
		return fmt.Sprint(n.n)
	case "var":
		if n.n < len(scope) {
			return scope[n.n]
		}
		return "0"
	case "add", "sub", "mul":
		return "(" + render(n.kids[0], scope) + " " + n.op + " " + render(n.kids[1], scope) + ")"
	case "cmp":
		return "(" + render(n.kids[0], scope) + " " + n.cmp + " " + render(n.kids[1], scope) + ")"
	case "if":
		return "(if " + render(n.kids[0], scope) + " [" + render(n.kids[1], scope) + "] [" + render(n.kids[2], scope) + "])"
	case "size":
		return "(size " + render(n.kids[0], scope) + ")"
	case "get":
		key := mapKeys[0]
		if n.n < len(n.kids[0].keys) {
			key = n.kids[0].keys[n.n]
		}
		return "(" + render(n.kids[0], scope) + " get " + key + ")"
	case "for":
		return "(for " + render(n.kids[0], scope) + " [" + render(n.kids[1], append(append([]string{}, scope...), "i")) + "])"
	case "list":
		parts := make([]string, len(n.kids))
		for i, k := range n.kids {
			parts[i] = render(k, scope)
		}
		return "[" + strings.Join(parts, " ") + "]"
	case "map":
		parts := make([]string, len(n.kids))
		for i, k := range n.kids {
			parts[i] = n.keys[i] + ": " + render(k, scope)
		}
		return "{" + strings.Join(parts, " ") + "}"
	case "def":
		return "def v" + fmt.Sprint(n.n) + " " + render(n.kids[0], scope)
	case "seq":
		parts := make([]string, len(n.kids))
		for i, k := range n.kids {
			parts[i] = render(k, scope)
		}
		return strings.Join(parts, " ")
	}
	return "0"
}

// diverges runs the program on both engines and reports whether the COMPILED
// path was taken and whether the two results disagree (value or error presence
// — the TestSpecCompiledDifferential invariant).
func diverges(t *testing.T, src string) (compiled, bad bool) {
	ac := newDifferentialInstance(t)
	gotC, comp, errC := ac.RunCompiled(src)
	if !comp {
		return false, false
	}
	ai := newDifferentialInstance(t)
	gotI, errI := ai.Run(src)
	if (errC == nil) != (errI == nil) {
		return true, true
	}
	if errC != nil {
		return true, false // both errored — presence matches
	}
	return true, fmt.Sprint(gotC) != fmt.Sprint(gotI)
}

func TestPropertyDifferential(t *testing.T) {
	if testing.Short() {
		t.Skip("property fuzz: skipped in -short")
	}
	// Lean DETERMINISTIC default (a regression guard, ~10s) — the same programs
	// every run, so a failure always reproduces. Override the budget for a deep
	// exploratory hunt: `AQL_FUZZ_SEEDS=20 AQL_FUZZ_ITERS=5000 go test -run
	// TestPropertyDifferential`.
	const depth = 4
	iters := fuzzEnv("AQL_FUZZ_ITERS", 1500)
	seeds := fuzzEnv("AQL_FUZZ_SEEDS", 2)
	var compiled, failures int
	for seed := 1; seed <= seeds; seed++ {
		r := rand.New(rand.NewSource(int64(seed)))
		for i := 0; i < iters; i++ {
			root, scope := genProgram(r, depth)
			src := render(root, scope)
			comp, bad := diverges(t, src)
			if comp {
				compiled++
			}
			if bad {
				failures++
				min := shrink(t, root, scope)
				t.Errorf("DIVERGENCE (seed %d iter %d):\n  full : %s\n  min  : %s", seed, i, src, render(min, scope))
				if failures >= 10 {
					t.Fatalf("stopping after %d divergences", failures)
				}
			}
		}
	}
	t.Logf("property fuzz: %d seeds x %d programs, %d took the compiled path, %d divergences", seeds, iters, compiled, failures)
}

// shrink greedily replaces sub-nodes with the minimal node of their category
// (and drops list elements) while the divergence persists, yielding a small
// reproducer. Bounded so a pathological case can't loop forever.
func shrink(t *testing.T, root *gnode, scope []string) *gnode {
	cur := root
	for round := 0; round < 200; round++ {
		progressed := false
		for _, cand := range simplifications(cur) {
			if _, bad := diverges(t, render(cand, scope)); bad {
				cur = cand
				progressed = true
				break
			}
		}
		if !progressed {
			return cur
		}
	}
	return cur
}

func minNode(c cat) *gnode {
	switch c {
	case cBool:
		return &gnode{op: "cmp", cat: cBool, cmp: "lt", kids: []*gnode{{op: "lit", cat: cInt}, {op: "lit", cat: cInt}}}
	case cList:
		return &gnode{op: "list", cat: cList, kids: []*gnode{{op: "lit", cat: cInt}}}
	case cMap:
		return &gnode{op: "map", cat: cMap, keys: []string{"a"}, kids: []*gnode{{op: "lit", cat: cInt}}}
	default:
		return &gnode{op: "lit", cat: cInt}
	}
}

// simplifications returns every single-point reduction of n: each reducible
// sub-node replaced by the minimal node of its category, plus list-element drops.
func simplifications(n *gnode) []*gnode {
	var out []*gnode
	var walk func(path []int)
	walk = func(path []int) {
		sub := at(n, path)
		// Replace sub with the minimal node of its category (if not already minimal).
		if !isMinimal(sub) {
			out = append(out, replace(n, path, minNode(sub.cat)))
		}
		// Drop one element from a multi-element list.
		if sub.op == "list" && len(sub.kids) > 1 {
			for d := range sub.kids {
				nl := &gnode{op: "list", cat: cList}
				for i, k := range sub.kids {
					if i != d {
						nl.kids = append(nl.kids, k)
					}
				}
				out = append(out, replace(n, path, nl))
			}
		}
		for i := range sub.kids {
			walk(append(append([]int{}, path...), i))
		}
	}
	walk(nil)
	return out
}

func isMinimal(n *gnode) bool {
	switch n.op {
	case "lit", "var":
		return true
	case "cmp":
		return n.kids[0].op == "lit" && n.kids[1].op == "lit"
	case "list":
		return len(n.kids) == 1 && n.kids[0].op == "lit"
	case "map":
		return len(n.kids) == 1 && n.kids[0].op == "lit"
	}
	return false
}

func at(n *gnode, path []int) *gnode {
	for _, i := range path {
		n = n.kids[i]
	}
	return n
}

// replace returns a deep copy of root with the node at path swapped for repl.
func replace(root *gnode, path []int, repl *gnode) *gnode {
	if len(path) == 0 {
		return repl
	}
	cp := *root
	cp.kids = append([]*gnode{}, root.kids...)
	cp.kids[path[0]] = replace(root.kids[path[0]], path[1:], repl)
	return &cp
}
