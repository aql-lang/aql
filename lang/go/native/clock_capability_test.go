package native

import (
	"testing"
	"time"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/capabilities"
)

// TestEffectiveClockDefault: with no clock installed, EffectiveClock
// returns a wall clock (never nil) whose Now is close to real time.
func TestEffectiveClockDefault(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	clk := EffectiveClock(r)
	if clk == nil {
		t.Fatal("EffectiveClock returned nil")
	}
	if d := time.Since(clk.Now()); d < -time.Minute || d > time.Minute {
		t.Errorf("default clock not near wall time: now=%v", clk.Now())
	}
}

// TestEffectiveClockFixed: an installed FixedClock is honored, so `now`
// becomes deterministic.
func TestEffectiveClockFixed(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	fixed := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	SetHostClock(r, capabilities.FixedClock{T: fixed})
	if got := EffectiveClock(r).Now(); !got.Equal(fixed) {
		t.Errorf("EffectiveClock.Now() = %v, want %v", got, fixed)
	}
	res, err := NewTop(r).Run(mustParseClock(t, "now"))
	if err != nil {
		t.Fatalf("run now: %v", err)
	}
	if got := res[len(res)-1].String(); got != "2021-01-01T00:00:00Z" {
		t.Errorf("now under FixedClock = %q, want 2021-01-01T00:00:00Z", got)
	}
}

func mustParseClock(t *testing.T, src string) []Value {
	t.Helper()
	v, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return v
}
