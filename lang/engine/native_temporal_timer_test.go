package engine_test

import (
	"github.com/metsitaba/voxgig-exp/lang/engine"
	"github.com/metsitaba/voxgig-exp/lang/native"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// sleep tests
// =============================================================================

func TestSleepBasic(t *testing.T) {
	reg, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	e := engine.NewTop(reg)
	start := time.Now()
	result, err := e.Run([]engine.Value{
		engine.NewInteger(50), engine.NewWord("sleep"),
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
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)
	_, err := e.Run([]engine.Value{
		engine.NewInteger(-1), engine.NewWord("sleep"),
	})
	if err == nil {
		t.Fatal("expected error for negative milliseconds")
	}
}

func TestSleepZero(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)
	result, err := e.Run([]engine.Value{
		engine.NewInteger(0), engine.NewWord("sleep"),
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
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)
	body := engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewWord("add"), engine.NewInteger(2)})
	body.Quoted = true
	result, err := e.Run([]engine.Value{
		engine.NewWord("timeout"), engine.NewInteger(100), body,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d: %v", len(result), result)
	}
	if !result[0].IsTimeout() {
		t.Fatalf("expected Timeout, got %s", result[0].VType)
	}
	ti, _ := result[0].AsTimeout()
	if ti.Timer != nil {
		ti.Timer.Stop()
	}
}

func TestTimeoutCallbackExecutes(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)

	// Register a custom word that sets an atomic flag when called.
	var flag atomic.Int32
	reg.RegisterStackOnly("testflag", engine.Signature{
		Args: []engine.Type{},
		Handler: func(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			flag.Store(1)
			return nil, nil
		},
	})

	e := engine.NewTop(reg)
	body := engine.NewList([]engine.Value{engine.NewWord("testflag")})
	body.Quoted = true
	result, err := e.Run([]engine.Value{
		engine.NewWord("timeout"), engine.NewInteger(20), body,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !result[0].IsTimeout() {
		t.Fatalf("expected Timeout result, got %v", result)
	}

	// Wait for the callback to fire.
	time.Sleep(100 * time.Millisecond)

	if flag.Load() != 1 {
		t.Error("expected callback to have executed")
	}
}

func TestTimeoutWithWordCallback(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)

	var flag atomic.Int32
	reg.RegisterStackOnly("testflag", engine.Signature{
		Args: []engine.Type{},
		Handler: func(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			flag.Store(1)
			return nil, nil
		},
	})

	e := engine.NewTop(reg)
	// timeout 20 testflag — word callback (quoted to atom)
	result, err := e.Run([]engine.Value{
		engine.NewWord("timeout"), engine.NewInteger(20), engine.NewAtom("testflag"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !result[0].IsTimeout() {
		t.Fatalf("expected Timeout result, got %v", result)
	}

	time.Sleep(100 * time.Millisecond)
	if flag.Load() != 1 {
		t.Error("expected word callback to have executed")
	}
}

func TestTimeoutNegativeErrors(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)
	body := engine.NewList([]engine.Value{engine.NewInteger(1)})
	body.Quoted = true
	_, err := e.Run([]engine.Value{
		engine.NewWord("timeout"), engine.NewInteger(-1), body,
	})
	if err == nil {
		t.Fatal("expected error for negative milliseconds")
	}
}

// =============================================================================
// interval tests
// =============================================================================

func TestIntervalReturnType(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)
	body := engine.NewList([]engine.Value{engine.NewInteger(1)})
	body.Quoted = true
	result, err := e.Run([]engine.Value{
		engine.NewWord("interval"), engine.NewInteger(100), body,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d: %v", len(result), result)
	}
	if !result[0].IsInterval() {
		t.Fatalf("expected Interval, got %s", result[0].VType)
	}
	ii, _ := result[0].AsInterval()
	if ii.Ticker != nil {
		ii.Ticker.Stop()
		close(ii.Done)
	}
}

func TestIntervalCallbackRepeats(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)

	var counter atomic.Int32
	reg.RegisterStackOnly("testinc", engine.Signature{
		Args: []engine.Type{},
		Handler: func(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			counter.Add(1)
			return nil, nil
		},
	})

	e := engine.NewTop(reg)
	body := engine.NewList([]engine.Value{engine.NewWord("testinc")})
	body.Quoted = true
	result, err := e.Run([]engine.Value{
		engine.NewWord("interval"), engine.NewInteger(20), body,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Let it tick several times.
	time.Sleep(120 * time.Millisecond)

	// Cancel.
	ii, _ := result[0].AsInterval()
	ii.Ticker.Stop()
	close(ii.Done)

	count := counter.Load()
	if count < 2 {
		t.Errorf("expected interval to tick at least twice, got %d", count)
	}
}

func TestIntervalZeroErrors(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)
	body := engine.NewList([]engine.Value{engine.NewInteger(1)})
	body.Quoted = true
	_, err := e.Run([]engine.Value{
		engine.NewWord("interval"), engine.NewInteger(0), body,
	})
	if err == nil {
		t.Fatal("expected error for zero milliseconds")
	}
}

// =============================================================================
// cancel tests
// =============================================================================

func TestCancelTimeout(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)

	var flag atomic.Int32
	reg.RegisterStackOnly("testflag", engine.Signature{
		Args: []engine.Type{},
		Handler: func(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			flag.Store(1)
			return nil, nil
		},
	})

	e := engine.NewTop(reg)
	body := engine.NewList([]engine.Value{engine.NewWord("testflag")})
	body.Quoted = true
	result, err := e.Run([]engine.Value{
		engine.NewWord("timeout"), engine.NewInteger(50), body,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Cancel immediately.
	e2 := engine.NewTop(reg)
	_, err = e2.Run([]engine.Value{result[0], engine.NewWord("cancel")})
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
	reg, _ := engine.DefaultRegistry(native.Register)

	var counter atomic.Int32
	reg.RegisterStackOnly("testinc", engine.Signature{
		Args: []engine.Type{},
		Handler: func(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			counter.Add(1)
			return nil, nil
		},
	})

	e := engine.NewTop(reg)
	body := engine.NewList([]engine.Value{engine.NewWord("testinc")})
	body.Quoted = true
	result, err := e.Run([]engine.Value{
		engine.NewWord("interval"), engine.NewInteger(20), body,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Let it tick a few times.
	time.Sleep(80 * time.Millisecond)

	// Cancel.
	e2 := engine.NewTop(reg)
	_, err = e2.Run([]engine.Value{result[0], engine.NewWord("cancel")})
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
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)
	body := engine.NewList([]engine.Value{engine.NewInteger(1)})
	body.Quoted = true
	result, err := e.Run([]engine.Value{
		engine.NewWord("timeout"), engine.NewInteger(1000), body,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Cancel twice should not panic.
	for i := 0; i < 2; i++ {
		e2 := engine.NewTop(reg)
		_, err = e2.Run([]engine.Value{result[0], engine.NewWord("cancel")})
		if err != nil {
			t.Fatalf("cancel #%d error: %v", i+1, err)
		}
	}
}

// =============================================================================
// Timeout/Interval String() tests
// =============================================================================

func TestTimeoutString(t *testing.T) {
	info := &engine.TimeoutInfo{ID: "T_test12345678", Ms: 100}
	v := engine.NewTimeout(info)
	s := v.String()
	if s != "Timeout(T_test12345678,100ms)" {
		t.Errorf("got %q", s)
	}
}

func TestIntervalString(t *testing.T) {
	info := &engine.IntervalInfo{ID: "T_test12345678", Ms: 50}
	v := engine.NewInterval(info)
	s := v.String()
	if s != "Interval(T_test12345678,50ms)" {
		t.Errorf("got %q", s)
	}
}

// =============================================================================
// Type literal no-panic tests
// =============================================================================

func TestTimerTypeLiteralNoPanic(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	for _, word := range []string{"cancel"} {
		t.Run(word, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic in %s: %v", word, r)
				}
			}()
			e := engine.NewTop(reg)
			_, _ = e.Run([]engine.Value{engine.NewTypeLiteral(engine.TTimeout), engine.NewWord(word)})
		})
	}
}
