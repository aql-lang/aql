package run

import (
	"testing"

	"github.com/aql-lang/aql/lang/go/capabilities"
)

// Every CLI driver must install the terminal probe, or IO.is-tty answers
// false everywhere and the word is useless. OptionsFor is the shared
// assembler `aql do` and `aql test` go through, so pinning it here covers
// both — and it is easy to forget, because a probe that is merely absent
// produces a plausible answer rather than a failure.
func TestOptionsForCarriesStreamProbe(t *testing.T) {
	o := OptionsFor("", 0, nil)
	if o.Streams == nil {
		t.Fatal("OptionsFor carries no StreamProbe: aql do / aql test would answer IO.is-tty false on a real terminal")
	}
	if _, ok := o.Streams.(capabilities.OSStreamProbe); !ok {
		t.Errorf("OptionsFor's probe = %T, want capabilities.OSStreamProbe", o.Streams)
	}
}
