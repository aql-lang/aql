package modules

import (
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// TestW8EmitAutoEncodeError drives emitAutoHandler's encode-error arms
// (emitlang.go:301 + 302-304): a value whose natural kind is csv (Parent
// conforms to Table) but whose payload is not a materializable table, so the
// csv encoder raises a coded emit_unsupported error that emit_auto re-raises
// under its own code.
func TestW8EmitAutoEncodeError(t *testing.T) {
	r := mcovReg(t)
	// A Table-typed value carrying an opaque ExtensionPayload the csv encoder
	// cannot materialize.
	v := eng.NewExtension(native.TTable, "w8-not-a-qb")
	if kind, ok := native.NaturalEmitKind(v); !ok || kind != "csv" {
		t.Fatalf("precondition: natural kind = %q, %v; want csv", kind, ok)
	}
	if _, err := emitAutoHandler([]native.Value{v, s7bMap()}, nil, nil, r); err == nil {
		t.Error("emit_auto: an unmaterializable csv value should raise")
	}
}
