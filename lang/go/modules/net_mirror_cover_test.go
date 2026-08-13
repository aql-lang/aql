package modules

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// Direct unit coverage for the boru:net check-mode mirrors
// (netAddrMirror, connectCodecMirror). The corpus drives the flagged
// shapes end to end; these tests own the arms a row cannot reach — the
// non-map decline, and the address parse that only runs once a codec is
// present.

func mirrorTestRegistry(t *testing.T) *native.Registry {
	t.Helper()
	reg := newCodecTestRegistry(t)
	t.Cleanup(reg.Check.Begin())
	return reg
}

// codecShapeMap is the minimum a codec must be for validateCodecShape to
// pass it on: a map carrying decode: and encode:. The entries' own types
// are not this validator's business — resolveCodec stamps them, and that
// half deliberately does not run during analysis.
func codecShapeMap() native.Value {
	m := native.NewOrderedMap()
	m.Set("decode", native.NewInteger(1))
	m.Set("encode", native.NewInteger(1))
	return native.NewMap(m)
}

func netOptsMap(kv ...native.Value) native.Value {
	m := native.NewOrderedMap()
	for i := 0; i+1 < len(kv); i += 2 {
		k, _ := kv[i].AsConcreteString()
		m.Set(k, kv[i+1])
	}
	return native.NewMap(m)
}

func TestConnectCodecMirrorArms(t *testing.T) {
	rf := connectCodecMirror()

	// A codec-less options map is the flagged shape: the runtime refuses
	// before dialling, so the mirror reports it with the same code.
	r := mirrorTestRegistry(t)
	out := rf([]native.Value{netOptsMap(native.NewString("tcp"), native.NewString("127.0.0.1:1"))}, r)
	if len(out) != 1 {
		t.Fatalf("residual = %v, want one value", out)
	}
	if len(r.Check.Diagnostics) != 1 || r.Check.Diagnostics[0].Code != "net_error" {
		t.Fatalf("a codec-less connect must flag net_error, got %+v", r.Check.Diagnostics)
	}

	// WITH a WELL-SHAPED codec, the mirror proceeds to the address parse.
	// A good address is silent.
	//
	// The codec has to be a real {decode encode} map: connectHandler
	// resolves it before dialling, so a String here is a guaranteed
	// runtime refusal, not a well-formed call. (This fixture WAS a
	// String, and passed only because the mirror skipped resolveCodec's
	// prefix — the gap the handler-order audit closed.)
	r2 := mirrorTestRegistry(t)
	ok := netOptsMap(
		native.NewString("tcp"), native.NewString("127.0.0.1:1"),
		native.NewString("codec"), codecShapeMap(),
	)
	rf([]native.Value{ok}, r2)
	if len(r2.Check.Diagnostics) != 0 {
		t.Fatalf("a well-formed connect must stay silent, got %+v", r2.Check.Diagnostics)
	}

	// ...and a malformed address is flagged with parseNetAddr's own
	// message, which is the point of reusing the handler's validator.
	r3 := mirrorTestRegistry(t)
	bad := netOptsMap(
		native.NewString("tcp"), native.NewInteger(99), // connect wants "host:port"
		native.NewString("codec"), codecShapeMap(),
	)
	rf([]native.Value{bad}, r3)
	if len(r3.Check.Diagnostics) != 1 || r3.Check.Diagnostics[0].Code != "net_error" {
		t.Fatalf("a malformed connect address must flag net_error, got %+v", r3.Check.Diagnostics)
	}

	// The codec's SHAPE is checked before the address, matching
	// connectHandler's order — so when both are wrong the codec wins.
	// A mirror that parsed the address first named the wrong error.
	r5 := mirrorTestRegistry(t)
	bothBad := netOptsMap(
		native.NewString("tcp"), native.NewInteger(99),
		native.NewString("codec"), native.NewInteger(5),
	)
	rf([]native.Value{bothBad}, r5)
	if len(r5.Check.Diagnostics) != 1 ||
		r5.Check.Diagnostics[0].Detail != "connect: codec: must be a {decode encode} map" {
		t.Fatalf("a bad codec must be named before the address, got %+v", r5.Check.Diagnostics)
	}

	// A codec map missing encode: is the other half of the shape rule.
	r6 := mirrorTestRegistry(t)
	halfCodec := native.NewOrderedMap()
	halfCodec.Set("decode", native.NewInteger(1))
	partial := netOptsMap(
		native.NewString("tcp"), native.NewString("127.0.0.1:1"),
		native.NewString("codec"), native.NewMap(halfCodec),
	)
	rf([]native.Value{partial}, r6)
	if len(r6.Check.Diagnostics) != 1 ||
		r6.Check.Diagnostics[0].Detail != "connect: codec must carry decode: and encode: functions" {
		t.Fatalf("a half codec must flag the decode/encode rule, got %+v", r6.Check.Diagnostics)
	}

	// A non-map operand passes the deep-concrete gate (an Integer has no
	// interior to disprove) but is not readable as options: the mirror
	// declines and leaves the refusal to dispatch, which owns the
	// signature mismatch.
	r4 := mirrorTestRegistry(t)
	rf([]native.Value{native.NewInteger(1)}, r4)
	if len(r4.Check.Diagnostics) != 0 {
		t.Fatalf("a non-map operand must be left to dispatch, got %+v", r4.Check.Diagnostics)
	}
}

func TestNetAddrMirrorArms(t *testing.T) {
	listen := netAddrMirror("listen", true, TListener)

	// The flagged shape: no tcp: and no unix:.
	r := mirrorTestRegistry(t)
	listen([]native.Value{native.NewMap(native.NewOrderedMap())}, r)
	if len(r.Check.Diagnostics) != 1 || r.Check.Diagnostics[0].Code != "net_error" {
		t.Fatalf("an address-less listen must flag net_error, got %+v", r.Check.Diagnostics)
	}

	// A valid port is silent, and a computed field closes the gate before
	// the validator can misread the carrier as a malformed port.
	r2 := mirrorTestRegistry(t)
	listen([]native.Value{netOptsMap(native.NewString("tcp"), native.NewInteger(0))}, r2)
	if len(r2.Check.Diagnostics) != 0 {
		t.Fatalf("a valid listen must stay silent, got %+v", r2.Check.Diagnostics)
	}
	r3 := mirrorTestRegistry(t)
	listen([]native.Value{netOptsMap(native.NewString("tcp"), core.NewCarrier(core.TInteger))}, r3)
	if len(r3.Check.Diagnostics) != 0 {
		t.Fatalf("a computed port must not be flagged, got %+v", r3.Check.Diagnostics)
	}
}
