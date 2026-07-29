// live_handle_example_test.go pins the one rule that keeps dynamic help
// from leaking goroutines.
//
// Example generation EXECUTES the expressions it synthesises, so that
// `describe` can show a real result. For a word that returns a LIVE
// HANDLE that is a resource leak with no owner: registering `interval`
// started a real ticker, nothing ever cancelled it, and its callback then
// ran on a forked registry for the lifetime of the process.
//
// The consequence was a data race, not a cosmetic one — it is what kept
// `make test-race` red, and any embedder registering the temporal module
// after MarkReady() leaked a goroutine per registration.
//
// These tests pin the contract at the seam rather than by counting
// goroutines, which would be timing-dependent: eval must never be called
// for such a word, and must still be called for everything else.
package help

import "testing"

func TestGenerateDynamicExamplesSkipsLiveHandleWords(t *testing.T) {
	for _, word := range []string{"interval", "timeout", "spawn", "service", "watch"} {
		t.Run(word, func(t *testing.T) {
			called := 0
			info := FuncInfo{
				Name: word,
				Sigs: []SigInfo{{Args: []string{"Integer", "Atom"}, BarrierPos: -1}},
			}
			GenerateDynamicExamples(info, func(string) (string, error) {
				called++
				return "x", nil
			})
			if called != 0 {
				t.Errorf("%s: eval ran %d time(s) — executing this word's example "+
					"creates a live timer/process the generator cannot cancel",
					word, called)
			}
		})
	}
}

// The negative half, and the one that keeps the rule honest: a skip list
// that grew to cover ordinary words would silently stop `describe` showing
// real results, which is the whole point of dynamic examples.
func TestGenerateDynamicExamplesStillRunsPureWords(t *testing.T) {
	for _, word := range []string{"add", "sub", "size", "upper"} {
		t.Run(word, func(t *testing.T) {
			called := 0
			info := FuncInfo{
				Name: word,
				Sigs: []SigInfo{{Args: []string{"Integer", "Integer"}, BarrierPos: -1}},
			}
			GenerateDynamicExamples(info, func(string) (string, error) {
				called++
				return "", nil // empty result: not stored, but eval DID run
			})
			if called == 0 {
				t.Errorf("%s: eval never ran — a pure word must still get its "+
					"example executed for real", word)
			}
		})
	}
}

func TestProducesLiveHandle(t *testing.T) {
	for _, w := range []string{"interval", "timeout", "spawn", "service", "watch"} {
		if !producesLiveHandle(w) {
			t.Errorf("%s returns a live handle and must be skipped", w)
		}
	}
	for _, w := range []string{"add", "get", "each", "now", "sleep", "cancel"} {
		if producesLiveHandle(w) {
			t.Errorf("%s is not a live-handle word and must keep its example; "+
				"note `now`/`sleep`/`cancel` are deliberately absent from the "+
				"list — they touch time but leave nothing running", w)
		}
	}
}
