package native

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Seam-9 coverage (W9_nativeC) for native_bytes.go: the pack/unpack
// segment-machinery error arms and helpers that native_bytes_cov_test.go
// does not reach. Pure helpers are driven directly with crafted bitSeg /
// fields values (design/TEST-SEAMS.10.md).

func w9BytesReg(t *testing.T) *Registry {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func w9EmptyFields(t *testing.T) ReadMap {
	t.Helper()
	f, err := RequireConcreteMap(NewMap(NewOrderedMap()), "t")
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// clampRange: the start<0 normalisation arm (331.15,333.3).
func TestW9ClampRangeNegativeStart(t *testing.T) {
	s, e := clampRange(-1, 2, 5)
	if s != 0 || e != 2 {
		t.Fatalf("clampRange(-1,2,5) = (%d,%d), want (0,2)", s, e)
	}
}

// readOneSeg: the explicit endian:'be' case (546.13,547.22), reached
// through refine BinarySpec.
func TestW9ReadSegEndianBE(t *testing.T) {
	r := seam5Reg(t)
	res, err := seam5Run(r, `def P (refine BinarySpec [{name:'x' type:'u16' endian:'be'}]) convert Bytes (make P {x:1})`)
	if err != nil || len(res) == 0 {
		t.Fatalf("endian:be pack: %v / %v", res, err)
	}
	// big-endian u16 of 1 is 00 01
	if s := res[len(res)-1].String(); !strings.Contains(s, "00 01") {
		t.Fatalf("endian:be pack got %q, want big-endian 00 01", s)
	}
}

// segSize / checkSegLen: the no-size ("the rest") short-circuits
// (587.18,589.3 and 750.18,752.3).
func TestW9SegSizeAndCheckSegLenNoSize(t *testing.T) {
	r := w9BytesReg(t)
	fields := w9EmptyFields(t)

	n, err := segSize(bitSeg{hasSize: false}, fields, r, "w")
	if err != nil || n != -1 {
		t.Fatalf("segSize(no-size) = (%d,%v), want (-1,nil)", n, err)
	}
	if err := checkSegLen(bitSeg{hasSize: false}, fields, 5, r, "w"); err != nil {
		t.Fatalf("checkSegLen(no-size) = %v, want nil", err)
	}
}

// packSeg: the segSize-error propagation for pad and bits segments sized
// by a field name that is not provided (808.17,810.4 and 815.17,817.4).
func TestW9PackSegSizeErrorProps(t *testing.T) {
	r := w9BytesReg(t)
	fields := w9EmptyFields(t)

	padSeg := bitSeg{typ: "pad", hasSize: true, sizeName: "nope"}
	if err := packSeg(&bitWriter{}, padSeg, fields, r, "w"); err == nil ||
		!strings.Contains(err.Error(), "not provided") {
		t.Fatalf("packSeg(pad, missing size field) = %v", err)
	}
	bitsSeg := bitSeg{typ: "bits", hasSize: true, sizeName: "nope"}
	if err := packSeg(&bitWriter{}, bitsSeg, fields, r, "w"); err == nil ||
		!strings.Contains(err.Error(), "not provided") {
		t.Fatalf("packSeg(bits, missing size field) = %v", err)
	}
}

// decodeSegs: the pad/bits resolveSize-error and pad short-buffer arms
// (983.16,985.5 / 987.11,989.5 / 998.18,1000.5).
func TestW9DecodeSegsErrorArms(t *testing.T) {
	r := w9BytesReg(t)

	// pad sized by a name that is not an earlier field.
	padName := []bitSeg{{typ: "pad", hasSize: true, sizeName: "nope"}}
	if _, _, err := decodeSegs([]byte{1}, padName, r, "w", false); err == nil ||
		!strings.Contains(err.Error(), "not an earlier field") {
		t.Fatalf("decodeSegs(pad, unknown size field) = %v", err)
	}

	// pad with a literal size but too few bytes → short buffer.
	padShort := []bitSeg{{typ: "pad", hasSize: true, sizeLit: 8}}
	if _, _, err := decodeSegs(nil, padShort, r, "w", false); err == nil {
		t.Fatalf("decodeSegs(pad, empty buffer) = nil, want short error")
	}

	// bits sized by a name that is not an earlier field.
	bitsName := []bitSeg{{typ: "bits", hasSize: true, sizeName: "nope"}}
	if _, _, err := decodeSegs([]byte{1}, bitsName, r, "w", false); err == nil ||
		!strings.Contains(err.Error(), "not an earlier field") {
		t.Fatalf("decodeSegs(bits, unknown size field) = %v", err)
	}
}

// init() export closure: core.ToNative of a Bytes-typed value whose payload
// is not []byte declines via the !ok arm (44.11,46.5).
func TestW9BytesBridgeExportDeclines(t *testing.T) {
	foreign := core.NewValueRaw(TBytes, &TimeoutInfo{})
	if out, ok := core.ToNative(foreign).([]byte); ok {
		t.Fatalf("ToNative(Bytes with foreign payload) = %v, want decline", out)
	}
}
