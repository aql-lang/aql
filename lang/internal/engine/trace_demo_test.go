package engine_test
import (
	"github.com/metsitaba/voxgig-exp/lang/internal/engine"
	"github.com/metsitaba/voxgig-exp/lang/internal/native"
	"os"
	"testing"
)

func TestTraceDemo(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	r.Output = os.Stderr // so it shows with -v

	// trace [1 add 2 mul 3]
	e := engine.NewTop(r)
	result, err := e.Run([]engine.Value{
		engine.NewWord("trace"),
		engine.NewList([]engine.Value{
			engine.NewInteger(1), engine.NewWord("add"), engine.NewInteger(2), engine.NewWord("mul"), engine.NewInteger(3),
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as0, _ := result[0].AsInteger()
	if len(result) != 1 || _as0 != 9 {
		t.Errorf("got %v, want [9]", result)
	}
}

func TestTraceDemoStringOps(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	r.Output = os.Stderr

	// trace ["hello" upper add " WORLD"]
	e := engine.NewTop(r)
	result, err := e.Run([]engine.Value{
		engine.NewWord("trace"),
		engine.NewList([]engine.Value{
			engine.NewString("hello"), engine.NewWord("upper"), engine.NewWord("add"), engine.NewString(" WORLD"),
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as1, _ := result[0].AsString()
	if len(result) != 1 || _as1 != "HELLO WORLD" {
		t.Errorf("got %v, want [HELLO WORLD]", result)
	}
}

func TestTraceDemoStackOps(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	r.Output = os.Stderr

	// trace [1 2 3 rot add mul]
	e := engine.NewTop(r)
	result, err := e.Run([]engine.Value{
		engine.NewWord("trace"),
		engine.NewList([]engine.Value{
			engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3),
			engine.NewWord("rot"), engine.NewWord("add"), engine.NewWord("mul"),
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_as2, _ := result[0].AsInteger()
	if len(result) != 1 || _as2 != 8 {
		t.Errorf("got %v, want [8]", result)
	}
}
