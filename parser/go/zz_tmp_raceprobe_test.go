package parser

// TEMP PROBE FILE — delete before finishing. Measures whether jsonic
// construction is data-race-free WITHOUT boru's jsonicMakeMu, on the
// upgraded deps (tabnas/jsonic/go v0.6.0, tabnas/parser/go v0.8.0).

import (
	"strings"
	"sync"
	"testing"

	jsonic "github.com/tabnas/jsonic/go"
)

// A: the bare dependency, no boru lock anywhere in the picture. This is the
// exact shape that tripped the detector before the upgrade.
func TestTmpBareJsonicMakeConcurrent(t *testing.T) {
	const goroutines, iters = 64, 24
	ids := make([]string, 0, goroutines*iters)
	var idmu sync.Mutex
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			local := make([]string, 0, iters)
			for i := 0; i < iters; i++ {
				var j *jsonic.Jsonic
				switch i % 3 {
				case 0:
					j = jsonic.Make()
				case 1:
					j = jsonic.Make(jsonic.Options{})
				default:
					j = jsonic.Make(jsonic.Options{
						Info:  &jsonic.InfoOptions{Text: boolPtr(true), List: boolPtr(true), Map: boolPtr(true)},
						List:  &jsonic.ListOptions{Pair: boolPtr(true), Child: boolPtr(true)},
						Map:   &jsonic.MapOptions{Child: boolPtr(true)},
						Value: &jsonic.ValueOptions{Lex: boolPtr(false)},
					})
				}
				if j == nil {
					t.Errorf("nil parser")
					return
				}
				local = append(local, j.Id())
				if _, err := j.Parse("a:1,b:[2,3]"); err != nil {
					t.Errorf("parse: %v", err)
					return
				}
			}
			idmu.Lock()
			ids = append(ids, local...)
			idmu.Unlock()
		}(g)
	}
	wg.Wait()
	// The counter's whole job is unique instance ids. A lost update under
	// concurrency shows up here as a duplicate even when the detector is off.
	seen := make(map[string]bool, len(ids))
	dups := 0
	for _, id := range ids {
		if seen[id] {
			dups++
		}
		seen[id] = true
	}
	t.Logf("constructed %d parsers, %d unique ids, %d duplicates", len(ids), len(seen), dups)
	if dups != 0 {
		t.Errorf("duplicate instance ids: %d of %d (counter lost updates)", dups, len(ids))
	}
}

// B: boru's REAL construction paths, concurrently. Under the lock-free
// overlay this is genuinely parallel construction of the full boru grammar.
func TestTmpBoruConstructionConcurrent(t *testing.T) {
	srcs := []string{
		"a: 1",
		"print 'hi'",
		"[1,2,3]",
		"m: {x: 1, y: [2, 3]}",
		"a: `tpl ${1} end`",
		"0xFF",
		"1_000",
	}
	const goroutines = 40
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			src := srcs[g%len(srcs)]
			if _, err := Parse(src); err != nil {
				t.Errorf("Parse(%q): %v", src, err)
			}
			if _, ok := LexTokens(src); !ok {
				t.Errorf("LexTokens(%q): lex error", src)
			}
			if _, err := ParseConfig("a: 1, b: two"); err != nil {
				t.Errorf("ParseConfig: %v", err)
			}
			if _, err := SafeParse("a:1"); err != nil {
				t.Errorf("SafeParse: %v", err)
			}
			if _, err := SafeParseData("42.0"); err != nil {
				t.Errorf("SafeParseData: %v", err)
			}
			_ = GuardMake(func() *jsonic.Jsonic { return jsonic.Make(jsonic.Options{}) })
			if _, err := SafeMake(jsonic.Options{}).Parse("k:v"); err != nil {
				t.Errorf("SafeMake: %v", err)
			}
		}(g)
	}
	wg.Wait()
}

// C: does jsonic.Parse's memoized singleton (sync.Once + shared instance)
// now coexist with concurrent Make? The SafeParse doc comment claims it
// "cannot be left to race with SafeMake".
func TestTmpJsonicParseSingletonVsMakeConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for g := 0; g < 48; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			if g%2 == 0 {
				if _, err := jsonic.Parse("a:1,b:2"); err != nil {
					t.Errorf("jsonic.Parse: %v", err)
				}
				return
			}
			j := jsonic.Make()
			if _, err := j.Parse("c:3"); err != nil {
				t.Errorf("Make+Parse: %v", err)
			}
		}(g)
	}
	wg.Wait()
}

// D: does the ordered-map / grammar build path touch any other global?
// Hammer construction with the widest option surface boru uses plus the
// declarative grammar load, and read back a full parse result.
func TestTmpWidestConstructionConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	errs := make(chan string, 64)
	for g := 0; g < 32; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vs, err := Parse("a: {b: [1, 'two', three], c: 0xFF}, d: 1_000")
			if err != nil {
				errs <- err.Error()
				return
			}
			if len(vs) == 0 {
				errs <- "empty parse"
			}
		}()
	}
	wg.Wait()
	close(errs)
	var got []string
	for e := range errs {
		got = append(got, e)
	}
	if len(got) != 0 {
		t.Errorf("concurrent Parse errors: %s", strings.Join(got, " | "))
	}
}
