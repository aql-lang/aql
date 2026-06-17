package native

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// A faithful port of patrun's trie matcher, for a head-to-head performance
// comparison against the shipped flat matcher (patrunBestMatch). The real
// patrun module can't be fetched here (network egress is locked down), so this
// reimplements its documented structure: each node branches on ONE property
// (keys consumed in sorted order); `exact` follows a pattern that constrains
// that key, `star` follows one that does not (the skip path for disjoint key
// sets). Find descends both and keeps the most-specific data node.
//
// TestPatrunTrieMatchesFlat fuzzes the two matchers against identical random
// tables/subjects and asserts they always agree — so the trie is a faithful
// stand-in (the flat matcher itself is pinned to patrun semantics by
// lang/spec/patrun.tsv), and the benchmark numbers are comparing like with
// like.

type ptNode struct {
	key   string             // property this node branches on ("" = none yet)
	exact map[string]*ptNode // value -> child (pattern HAS key=value)
	star  *ptNode            // pattern does NOT constrain key (skip)
	data  Value
	has   bool
	dkeys []string // a terminating pattern's sorted keys (for tie-break)
}

type ptTrie struct{ root *ptNode }

func newPtTrie() *ptTrie { return &ptTrie{root: &ptNode{}} }

func (t *ptTrie) add(sortedKeys []string, pat map[string]string, data Value) {
	t.root = ptInsert(t.root, sortedKeys, 0, pat, data)
}

func ptInsert(node *ptNode, keys []string, i int, pat map[string]string, data Value) *ptNode {
	if node == nil {
		node = &ptNode{}
	}
	if i >= len(keys) {
		node.data, node.has, node.dkeys = data, true, keys
		return node
	}
	k := keys[i]
	if node.key == "" {
		node.key = k
	}
	switch {
	case k == node.key:
		if node.exact == nil {
			node.exact = map[string]*ptNode{}
		}
		v := pat[k]
		node.exact[v] = ptInsert(node.exact[v], keys, i+1, pat, data)
		return node
	case k > node.key:
		// pattern skips node.key
		node.star = ptInsert(node.star, keys, i, pat, data)
		return node
	default: // k < node.key — re-root on k; the old node becomes the skip-k branch
		nn := &ptNode{key: k, exact: map[string]*ptNode{}, star: node}
		nn.exact[pat[k]] = ptInsert(nil, keys, i+1, pat, data)
		return nn
	}
}

func (t *ptTrie) find(subj map[string]string) (Value, bool) {
	best := ptFind(t.root, subj)
	if best == nil {
		return Value{}, false
	}
	return best.data, true
}

func ptFind(node *ptNode, subj map[string]string) *ptNode {
	if node == nil {
		return nil
	}
	var best *ptNode
	if node.has {
		best = node
	}
	if node.key != "" {
		if v, ok := subj[node.key]; ok {
			best = ptBetter(best, ptFind(node.exact[v], subj))
		}
		best = ptBetter(best, ptFind(node.star, subj))
	}
	return best
}

// ptBetter applies the SAME ordering as the flat matcher's moreSpecific: more
// matched keys wins; on a tie the lexicographically smaller sorted key list
// wins. Identical ordering is what lets the fuzz assert exact agreement.
func ptBetter(a, b *ptNode) *ptNode {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	}
	if len(a.dkeys) != len(b.dkeys) {
		if len(a.dkeys) > len(b.dkeys) {
			return a
		}
		return b
	}
	for i := range a.dkeys {
		if a.dkeys[i] != b.dkeys[i] {
			if a.dkeys[i] < b.dkeys[i] {
				return a
			}
			return b
		}
	}
	return a
}

func ptSortedKeys(pat map[string]string) []string {
	ks := make([]string, 0, len(pat))
	for k := range pat {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func ptSig(keys []string, pat map[string]string) string {
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(pat[k])
	}
	return b.String()
}

// TestPatrunTrieMatchesFlat fuzzes the trie against the shipped flat matcher
// on identical random tables and subjects; they must always agree (same
// matched value, or both no-match). This pins the trie as a faithful patrun
// stand-in for the benchmark.
func TestPatrunTrieMatchesFlat(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	keys := []string{"a", "b", "c", "d"}
	for iter := 0; iter < 5000; iter++ {
		flat := &patrunMatcher{}
		trie := newPtTrie()
		seen := map[string]bool{}
		for p := 0; p < 1+rng.Intn(12); p++ {
			pat := map[string]string{}
			for _, k := range keys {
				if rng.Intn(2) == 0 {
					pat[k] = strconv.Itoa(1 + rng.Intn(3))
				}
			}
			if len(pat) == 0 {
				pat["a"] = "1"
			}
			sk := ptSortedKeys(pat)
			sig := ptSig(sk, pat)
			if seen[sig] {
				continue
			}
			seen[sig] = true
			val := NewString(sig)
			flat.entries = append(flat.entries, &patrunEntry{keys: sk, pat: pat, val: val})
			trie.add(sk, pat, val)
		}
		subj := map[string]string{}
		for _, k := range keys {
			if rng.Intn(2) == 0 {
				subj[k] = strconv.Itoa(1 + rng.Intn(3))
			}
		}
		fe := patrunBestMatch(flat, subj, false)
		tv, tok := trie.find(subj)
		switch {
		case fe == nil && tok:
			t.Fatalf("iter %d: flat=nil but trie matched; subj=%v", iter, subj)
		case fe == nil:
			continue
		case !tok:
			fv, _ := AsString(fe.val)
			t.Fatalf("iter %d: flat=%q but trie=nil; subj=%v", iter, fv, subj)
		default:
			fv, _ := AsString(fe.val)
			tvs, _ := AsString(tv)
			if fv != tvs {
				t.Fatalf("iter %d: flat=%q trie=%q subj=%v", iter, fv, tvs, subj)
			}
		}
	}
}

func buildBenchTrie(n int) *ptTrie {
	tr := newPtTrie()
	for i := 0; i < n; i++ {
		s := strconv.Itoa(i)
		tr.add([]string{"cmd", "role"}, map[string]string{"cmd": "c" + s, "role": "r" + s}, NewString("v"+s))
	}
	return tr
}

// BenchmarkPatrunFindTrie mirrors BenchmarkPatrunFindMatcher (same workload)
// on the trie: find should be ~O(query-key-depth), independent of table size.
func BenchmarkPatrunFindTrie(b *testing.B) {
	for _, n := range []int{1, 10, 100, 1000, 10000} {
		tr := buildBenchTrie(n)
		last := strconv.Itoa(n - 1)
		subj := map[string]string{"role": "r" + last, "cmd": "c" + last, "x": "extra"}
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, ok := tr.find(subj); !ok {
					b.Fatal("expected a match")
				}
			}
		})
	}
}
