package native

import (
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// sleep tests
// =============================================================================

func TestSleepBasic(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := NewTop(reg)
	start := time.Now()
	result, err := e.Run([]Value{
		NewInteger(50), NewWord("sleep"),
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty stack, got %d values: %v", len(result), result)
	}
	if elapsed < 40*time.Millisecond {
		t.Errorf("sleep returned too quickly: %v", elapsed)
	}
}

func TestSleepNegativeErrors(t *testing.T) {
	reg, _ := DefaultRegistry()
	e := NewTop(reg)
	_, err := e.Run([]Value{
		NewInteger(-1), NewWord("sleep"),
	})
	if err == nil {
		t.Fatal("expected error for negative milliseconds")
	}
}

func TestSleepZero(t *testing.T) {
	reg, _ := DefaultRegistry()
	e := NewTop(reg)
	result, err := e.Run([]Value{
		NewInteger(0), NewWord("sleep"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty stack, got %d values", len(result))
	}
}

// =============================================================================
// timeout tests
// =============================================================================

func TestTimeoutReturnType(t *testing.T) {
	reg, _ := DefaultRegistry()
	e := NewTop(reg)
	body := NewList([]Value{NewInteger(1), NewWord("add"), NewInteger(2)})
	body.Quoted = true
	result, err := e.Run([]Value{
		NewWord("timeout"), NewInteger(100), body,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d: %v", len(result), result)
	}
	if !result[0].Parent.Equal(TTimeout) {
		t.Fatalf("expected Timeout, got %s", result[0].Parent)
	}
	ti, _ := result[0].Data.(*TimeoutInfo), true
	if ti.Timer != nil {
		ti.Timer.Stop()
	}
}

func TestTimeoutCallbackExecutes(t *testing.T) {
	reg, _ := DefaultRegistry()

	// Register a custom word that sets an atomic flag when called.
	var flag atomic.Int32
	reg.RegisterStackOnly("testflag", Signature{
		Args: []*Type{},
		Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			flag.Store(1)
			return nil, nil
		},
	})

	e := NewTop(reg)
	body := NewList([]Value{NewWord("testflag")})
	body.Quoted = true
	result, err := e.Run([]Value{
		NewWord("timeout"), NewInteger(20), body,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !result[0].Parent.Equal(TTimeout) {
		t.Fatalf("expected Timeout result, got %v", result)
	}

	// Wait for the callback to fire.
	time.Sleep(100 * time.Millisecond)

	if flag.Load() != 1 {
		t.Error("expected callback to have executed")
	}
}

func TestTimeoutWithWordCallback(t *testing.T) {
	reg, _ := DefaultRegistry()

	var flag atomic.Int32
	reg.RegisterStackOnly("testflag", Signature{
		Args: []*Type{},
		Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			flag.Store(1)
			return nil, nil
		},
	})

	e := NewTop(reg)
	// timeout 20 testflag — word callback (quoted to atom)
	result, err := e.Run([]Value{
		NewWord("timeout"), NewInteger(20), NewAtom("testflag"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !result[0].Parent.Equal(TTimeout) {
		t.Fatalf("expected Timeout result, got %v", result)
	}

	time.Sleep(100 * time.Millisecond)
	if flag.Load() != 1 {
		t.Error("expected word callback to have executed")
	}
}

func TestTimeoutNegativeErrors(t *testing.T) {
	reg, _ := DefaultRegistry()
	e := NewTop(reg)
	body := NewList([]Value{NewInteger(1)})
	body.Quoted = true
	_, err := e.Run([]Value{
		NewWord("timeout"), NewInteger(-1), body,
	})
	if err == nil {
		t.Fatal("expected error for negative milliseconds")
	}
}

// =============================================================================
// interval tests
// =============================================================================

func TestIntervalReturnType(t *testing.T) {
	reg, _ := DefaultRegistry()
	e := NewTop(reg)
	body := NewList([]Value{NewInteger(1)})
	body.Quoted = true
	result, err := e.Run([]Value{
		NewWord("interval"), NewInteger(100), body,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d: %v", len(result), result)
	}
	if !result[0].Parent.Equal(TInterval) {
		t.Fatalf("expected Interval, got %s", result[0].Parent)
	}
	ii, _ := result[0].Data.(*IntervalInfo), true
	if ii.Ticker != nil {
		ii.Ticker.Stop()
		close(ii.Done)
	}
}

func TestIntervalCallbackRepeats(t *testing.T) {
	reg, _ := DefaultRegistry()

	var counter atomic.Int32
	reg.RegisterStackOnly("testinc", Signature{
		Args: []*Type{},
		Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			counter.Add(1)
			return nil, nil
		},
	})

	e := NewTop(reg)
	body := NewList([]Value{NewWord("testinc")})
	body.Quoted = true
	result, err := e.Run([]Value{
		NewWord("interval"), NewInteger(20), body,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Let it tick several times.
	time.Sleep(120 * time.Millisecond)

	// Cancel.
	ii, _ := result[0].Data.(*IntervalInfo), true
	ii.Ticker.Stop()
	close(ii.Done)

	count := counter.Load()
	if count < 2 {
		t.Errorf("expected interval to tick at least twice, got %d", count)
	}
}

func TestIntervalZeroErrors(t *testing.T) {
	reg, _ := DefaultRegistry()
	e := NewTop(reg)
	body := NewList([]Value{NewInteger(1)})
	body.Quoted = true
	_, err := e.Run([]Value{
		NewWord("interval"), NewInteger(0), body,
	})
	if err == nil {
		t.Fatal("expected error for zero milliseconds")
	}
}

// =============================================================================
// cancel tests
// =============================================================================

func TestCancelTimeout(t *testing.T) {
	reg, _ := DefaultRegistry()

	var flag atomic.Int32
	reg.RegisterStackOnly("testflag", Signature{
		Args: []*Type{},
		Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			flag.Store(1)
			return nil, nil
		},
	})

	e := NewTop(reg)
	body := NewList([]Value{NewWord("testflag")})
	body.Quoted = true
	result, err := e.Run([]Value{
		NewWord("timeout"), NewInteger(50), body,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Cancel immediately.
	e2 := NewTop(reg)
	_, err = e2.Run([]Value{result[0], NewWord("cancel")})
	if err != nil {
		t.Fatalf("cancel error: %v", err)
	}

	// Wait and verify callback did NOT run.
	time.Sleep(100 * time.Millisecond)
	if flag.Load() != 0 {
		t.Error("expected callback NOT to run after cancel")
	}
}

func TestCancelInterval(t *testing.T) {
	reg, _ := DefaultRegistry()

	var counter atomic.Int32
	reg.RegisterStackOnly("testinc", Signature{
		Args: []*Type{},
		Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			counter.Add(1)
			return nil, nil
		},
	})

	e := NewTop(reg)
	body := NewList([]Value{NewWord("testinc")})
	body.Quoted = true
	result, err := e.Run([]Value{
		NewWord("interval"), NewInteger(20), body,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Let it tick a few times.
	time.Sleep(80 * time.Millisecond)

	// Cancel.
	e2 := NewTop(reg)
	_, err = e2.Run([]Value{result[0], NewWord("cancel")})
	if err != nil {
		t.Fatalf("cancel error: %v", err)
	}

	// Allow a brief settle after cancel for any in-flight tick to complete.
	time.Sleep(40 * time.Millisecond)
	countAfterCancel := counter.Load()

	// Wait and verify count stopped growing.
	time.Sleep(80 * time.Millisecond)
	countFinal := counter.Load()

	if countAfterCancel == 0 {
		t.Error("expected interval to have ticked at least once")
	}
	if countFinal != countAfterCancel {
		t.Errorf("expected count to stop at %d after cancel, but got %d", countAfterCancel, countFinal)
	}
}

func TestCancelIdempotent(t *testing.T) {
	reg, _ := DefaultRegistry()
	e := NewTop(reg)
	body := NewList([]Value{NewInteger(1)})
	body.Quoted = true
	result, err := e.Run([]Value{
		NewWord("timeout"), NewInteger(1000), body,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Cancel twice should not panic.
	for i := 0; i < 2; i++ {
		e2 := NewTop(reg)
		_, err = e2.Run([]Value{result[0], NewWord("cancel")})
		if err != nil {
			t.Fatalf("cancel #%d error: %v", i+1, err)
		}
	}
}

// =============================================================================
// Timeout/Interval String() tests
// =============================================================================

func TestTimeoutString(t *testing.T) {
	info := &TimeoutInfo{ID: "T_test12345678", Ms: 100}
	v := NewTimeout(info)
	s := v.String()
	if s != "Timeout(T_test12345678,100ms)" {
		t.Errorf("got %q", s)
	}
}

func TestIntervalString(t *testing.T) {
	info := &IntervalInfo{ID: "T_test12345678", Ms: 50}
	v := NewInterval(info)
	s := v.String()
	if s != "Interval(T_test12345678,50ms)" {
		t.Errorf("got %q", s)
	}
}

// =============================================================================
// Type literal no-panic tests
// =============================================================================

func TestTimerTypeLiteralNoPanic(t *testing.T) {
	reg, _ := DefaultRegistry()
	for _, word := range []string{"cancel"} {
		t.Run(word, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic in %s: %v", word, r)
				}
			}()
			e := NewTop(reg)
			_, _ = e.Run([]Value{NewTypeLiteral(TTimeout), NewWord(word)})
		})
	}
}
