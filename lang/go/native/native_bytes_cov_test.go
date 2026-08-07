package native

import (
	"errors"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	parser "github.com/boru-lang/boru/parser/go"
)

// Coverage tests for native_bytes.go: the Bytes scalar behaviors, the
// convert/slice/add overloads, and the binary-frame bit syntax
// (refine BinarySpec / make / convert Bytes / unpack / unpack-prefix).
// Everything runs through the language surface (parse + run), pairing
// each accepted form with the shapes that must be rejected.

func runBytesCov(t *testing.T, src string) string {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	out, err := NewTop(r).Run(toks)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return Canon(out)
}

func runBytesCovErr(t *testing.T, src string) error {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	_, runErr := NewTop(r).Run(toks)
	return runErr
}

func TestBytesCovConvertSliceAdd(t *testing.T) {
	cases := []struct{ src, want string }{
		// convert: String/List <-> Bytes, compacting Bytes -> Bytes
		{`convert Bytes "hi"`, "Bytes<68 69>"},
		{`convert String (convert Bytes [104 105])`, "'hi'"},
		{`convert List (convert Bytes [104 105])`, "[104 105]"},
		{`convert Bytes [104 105]`, "Bytes<68 69>"},
		{`convert Bytes (convert Bytes [104 105])`, "Bytes<68 69>"},
		{`convert List (convert Bytes [])`, "[]"},

		// size via the Sizer behavior
		{`size (convert Bytes [104 105])`, "2"},
		{`size (convert Bytes [])`, "0"},
		{`size (convert Bytes "café")`, "5"},

		// slice: clamping, start-only, all
		{`convert String (slice 1 3 (convert Bytes [104 101 108 108 111]))`, "'el'"},
		{`convert String (slice 0 99 (convert Bytes "hi"))`, "'hi'"},
		{`convert String (slice 3 1 (convert Bytes "hi"))`, "''"},
		{`convert String (slice 9 (convert Bytes "hi"))`, "''"},
		{`convert String (slice 1 (convert Bytes "hi"))`, "'i'"},
		{`convert String (slice (convert Bytes "hi"))`, "'hi'"},

		// add concatenates a ++ b
		{`convert String ((convert Bytes "ab") add (convert Bytes "cd"))`, "'abcd'"},
		{`convert String ((convert Bytes "") add (convert Bytes "x"))`, "'x'"},

		// equality / ordering / type-literal-first rule
		{`(convert Bytes "hi") eq (convert Bytes "hi")`, "true"},
		{`(convert Bytes "ho") eq (convert Bytes "hi")`, "false"},
		{`(convert Bytes "hi") eq 1`, "false"},
		{`(convert Bytes "a") lt (convert Bytes "b")`, "true"},
		{`(convert Bytes "b") lt (convert Bytes "a")`, "false"},
		{`(convert Bytes "a") cmp (convert Bytes "a")`, "0"},
		{`Bytes cmp (convert Bytes "hi")`, "-1"},
		{`(convert Bytes "hi") cmp Bytes`, "1"},
		{`(convert Bytes "hi") is Bytes`, "true"},
		{`(convert Bytes "hi") is String`, "false"},
	}
	for _, tc := range cases {
		if got := runBytesCov(t, tc.src); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// The hex render caps at 32 bytes and reports the full length.
func TestBytesCovFormatCap(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("convert Bytes [")
	for i := 0; i < 40; i++ {
		sb.WriteString("7 ")
	}
	sb.WriteString("]")
	got := runBytesCov(t, sb.String())
	if !strings.Contains(got, "… (40)") {
		t.Errorf("40-byte render must cap and report length, got %q", got)
	}
}

func TestBytesCovConvertErrors(t *testing.T) {
	cases := []struct{ src, code string }{
		{`convert Bytes [256]`, "expected-byte"},
		{`convert Bytes [(0 sub 1)]`, "expected-byte"},
		{`convert Bytes ['x']`, "expected-byte"},
		{`convert String (convert Bytes [255])`, "bad-encoding"},
	}
	for _, tc := range cases {
		err := runBytesCovErr(t, tc.src)
		if err == nil || !strings.Contains(err.Error(), tc.code) {
			t.Errorf("%s: error = %v, want code %q", tc.src, err, tc.code)
		}
	}
}

// Type literals in the data slots must be reported as errors, never panic.
func TestBytesCovTypeLiteralNoPanic(t *testing.T) {
	srcs := []string{
		`convert Bytes String`,
		`convert String Bytes`,
		`convert Bytes List`,
		`convert List Bytes`,
		`convert Bytes Bytes`,
		`slice 0 1 Bytes`,
		`slice 0 Bytes`,
		`slice Bytes`,
		`Bytes add Bytes`,
		`size Bytes`,
	}
	for _, src := range srcs {
		t.Run(src, func(t *testing.T) {
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("panic on %q: %v", src, rec)
				}
			}()
			r, err := DefaultRegistry()
			if err != nil {
				t.Fatal(err)
			}
			toks, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			_, _ = NewTop(r).Run(toks)
		})
	}
}

// Direct behavior probes for the shapes the language surface cannot produce.
func TestBytesCovBehaviorFallbacks(t *testing.T) {
	b := bytesBehavior{}
	if got := b.Format(NewInteger(1)); got != "Bytes<?>" {
		t.Errorf("Format(non-bytes) = %q, want Bytes<?>", got)
	}
	if got := b.Size(NewInteger(1)); got != 0 {
		t.Errorf("Size(non-bytes) = %d, want 0", got)
	}
	if _, err := b.Compare(newBytes([]byte("a")), NewInteger(1)); !errors.Is(err, core.ErrNoComparer) {
		t.Errorf("Compare(bytes, int) err = %v, want ErrNoComparer", err)
	}
	if _, err := b.Compare(NewTypeLiteral(TBytes), NewTypeLiteral(TBytes)); !errors.Is(err, core.ErrNoComparer) {
		t.Errorf("Compare(lit, lit) err = %v, want ErrNoComparer", err)
	}
	if _, ok := asBytes(Value{}); ok {
		t.Error("asBytes(zero Value) must fail")
	}
	if _, ok := asBytes(NewString("x")); ok {
		t.Error("asBytes(String) must fail")
	}
}

// ---- binary frames: spec validation ----------------------------------------

func TestBytesCovSpecValidationErrors(t *testing.T) {
	cases := []struct{ src, frag string }{
		{`refine BinarySpec [5]`, "is not a Map"},
		{`refine BinarySpec [{name:'x' value:1 type:'u8'}]`, "both"},
		{`refine BinarySpec [{name:5 type:'u8'}]`, "must be a String"},
		{`refine BinarySpec [{value:'x' type:'u8'}]`, "must be an Integer"},
		{`refine BinarySpec [{type:'u8'}]`, "needs a"},
		{`refine BinarySpec [{name:'x'}]`, "has no"},
		{`refine BinarySpec [{name:'x' type:5}]`, "must be a String"},
		{`refine BinarySpec [{name:'x' type:'nope'}]`, "unknown type"},
		{`refine BinarySpec [{value:1 type:'f32'}]`, "not valid on a f32"},
		{`refine BinarySpec [{value:1 type:'utf8'}]`, "not valid on a utf8"},
		{`refine BinarySpec [{name:'x' type:'u16' endian:'xx'}]`, "unknown endian"},
		{`refine BinarySpec [{name:'x' type:'u16' endian:5}]`, "must be a String"},
		{`refine BinarySpec [{name:'x' type:'u8' signed:5}]`, "must be a Boolean"},
		{`refine BinarySpec [{name:'x' type:'bytes' signed:true}]`, "not valid on bytes"},
		{`refine BinarySpec [{name:'x' type:'bytes' size:(0 sub 1)}]`, "must not be negative"},
		{`refine BinarySpec [{name:'x' type:'bytes' size:true}]`, "must be an Integer or a field-name String"},
		{`refine BinarySpec [{name:'x' type:'bits'}]`, "needs a `size`"},
		{`refine BinarySpec [{name:'x' type:'pad'}]`, "needs a `size`"},
	}
	for _, tc := range cases {
		err := runBytesCovErr(t, tc.src)
		if err == nil || !strings.Contains(err.Error(), tc.frag) {
			t.Errorf("%s: error = %v, want fragment %q", tc.src, err, tc.frag)
		}
	}
}

// ---- binary frames: pack (convert Bytes <instance>) ------------------------

func TestBytesCovPack(t *testing.T) {
	cases := []struct{ src, want string }{
		// constants only, integer widths, byte order
		{`def P (refine BinarySpec [{value:1 type:'u8'} {value:2 type:'u16'}]) convert Bytes (make P {})`, "Bytes<01 00 02>"},
		{`def P (refine BinarySpec [{name:'x' type:'u16' endian:'le'}]) convert Bytes (make P {x:1})`, "Bytes<01 00>"},
		{`def P (refine BinarySpec [{name:'x' type:'u64'}]) convert Bytes (make P {x:1})`, "Bytes<00 00 00 00 00 00 00 01>"},
		{`def P (refine BinarySpec [{name:'x' type:'i8'}]) convert Bytes (make P {x:(0 sub 1)})`, "Bytes<ff>"},
		// bits + pad
		{`def P (refine BinarySpec [{value:1 type:'bits' size:4} {value:5 type:'bits' size:4}]) convert Bytes (make P {})`, "Bytes<15>"},
		{`def P (refine BinarySpec [{name:'x' type:'bits' size:4} {name:'p' type:'pad' size:4}]) convert Bytes (make P {x:5})`, "Bytes<50>"},
		// bits sized by a field name
		{`def P (refine BinarySpec [{name:'n' type:'u8'} {name:'x' type:'bits' size:'n'}]) convert Bytes (make P {n:8 x:255})`, "Bytes<08 ff>"},
		// utf8 with a declared size, and a length-prefixed bytes payload
		{`def P (refine BinarySpec [{name:'s' type:'utf8' size:2}]) convert Bytes (make P {s:'hi'})`, "Bytes<68 69>"},
		{`def H (refine BinarySpec [{value:1 type:'u8'} {name:'len' type:'u16'} {name:'body' type:'bytes' size:'len'}]) convert Bytes (make H {len:2 body:(convert Bytes "hi")})`, "Bytes<01 00 02 68 69>"},
	}
	for _, tc := range cases {
		if got := runBytesCov(t, tc.src); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestBytesCovPackErrors(t *testing.T) {
	cases := []struct{ src, frag string }{
		// value range checks
		{`def P (refine BinarySpec [{name:'x' type:'u8'}]) convert Bytes (make P {x:300})`, "does not fit u8"},
		{`def P (refine BinarySpec [{name:'x' type:'u8'}]) convert Bytes (make P {x:(0 sub 1)})`, "does not fit u8"},
		{`def P (refine BinarySpec [{name:'x' type:'i8'}]) convert Bytes (make P {x:(0 sub 129)})`, "does not fit i8"},
		{`def P (refine BinarySpec [{name:'x' type:'bits' size:4} {name:'p' type:'pad' size:4}]) convert Bytes (make P {x:31})`, "does not fit a 4-bit field"},
		{`def P (refine BinarySpec [{name:'x' type:'bits' size:4} {name:'p' type:'pad' size:4}]) convert Bytes (make P {x:(0 sub 1)})`, "does not fit a 4-bit field"},
		// alignment
		{`def P (refine BinarySpec [{value:1 type:'bits' size:3}]) convert Bytes (make P {})`, "unaligned"},
		{`def P (refine BinarySpec [{value:1 type:'bits' size:4} {name:'b' type:'bytes'}]) convert Bytes (make P {b:(convert Bytes "x")})`, "not byte-aligned"},
		// size resolution at pack time
		{`def P (refine BinarySpec [{name:'b' type:'bytes' size:'nope'}]) convert Bytes (make P {b:(convert Bytes "hi")})`, "is not provided"},
		{`def P (refine BinarySpec [{name:'b' type:'bytes' size:2} {name:'c' type:'bytes' size:'b'}]) convert Bytes (make P {b:(convert Bytes "hi") c:(convert Bytes "xy")})`, "is not an Integer"},
		// declared-size consistency
		{`def P (refine BinarySpec [{name:'b' type:'bytes' size:3}]) convert Bytes (make P {b:(convert Bytes "hi")})`, "declared size is 3"},
		{`def P (refine BinarySpec [{name:'s' type:'utf8' size:3}]) convert Bytes (make P {s:'hi'})`, "declared size is 3"},
		// not a Binary instance / not a frame type
		{`def C class {a:1} convert Bytes (make C {})`, "type_error"},
	}
	for _, tc := range cases {
		err := runBytesCovErr(t, tc.src)
		if err == nil || !strings.Contains(err.Error(), tc.frag) {
			t.Errorf("%s: error = %v, want fragment %q", tc.src, err, tc.frag)
		}
	}
}

// ---- binary frames: unpack / unpack-prefix ---------------------------------

func TestBytesCovUnpack(t *testing.T) {
	cases := []struct{ src, want string }{
		// field decode + guard pass + size by earlier field
		{`def P (refine BinarySpec [{name:'ver' type:'u8'} {name:'len' type:'u16'} {name:'body' type:'bytes' size:'len'}]) def m (unpack P (convert Bytes [1 0 2 104 105])) convert String m.body`, "'hi'"},
		{`def P (refine BinarySpec [{value:127 type:'u8'} {name:'x' type:'u8'}]) def m (unpack P (convert Bytes [127 2])) m.x`, "2"},
		// bits + pad round trip
		{`def P (refine BinarySpec [{name:'x' type:'bits' size:4} {name:'p' type:'pad' size:4}]) def m (unpack P (convert Bytes [80])) m.x`, "5"},
		// literal-only frame decodes to a fieldless instance
		{`def P (refine BinarySpec [{value:1 type:'bits' size:4} {value:5 type:'bits' size:4}]) typeof (unpack P (convert Bytes [21]))`, "P"},
		// trailing "rest" bytes and utf8 segments
		{`def P (refine BinarySpec [{name:'b' type:'bytes'}]) def m (unpack P (convert Bytes "hi")) convert String m.b`, "'hi'"},
		{`def P (refine BinarySpec [{name:'s' type:'utf8'}]) def m (unpack P (convert Bytes "hé")) m.s`, "'hé'"},
		// endianness, sign handling, floats
		{`def P (refine BinarySpec [{name:'x' type:'u16' endian:'le'}]) def m (unpack P (convert Bytes [1 0])) m.x`, "1"},
		{`def S (refine BinarySpec [{name:'x' type:'i16'}]) def w (convert Bytes (make S {x:(0 sub 2)})) def m (unpack S w) m.x`, "-2"},
		{`def P (refine BinarySpec [{name:'x' type:'u8' signed:true}]) def m (unpack P (convert Bytes [255])) m.x`, "-1"},
		{`def F (refine BinarySpec [{name:'x' type:'f32'}]) def w (convert Bytes (make F {x:1.5})) def m (unpack F w) m.x`, "1.5"},
		{`def F (refine BinarySpec [{name:'x' type:'f64' endian:'le'}]) def w (convert Bytes (make F {x:1.5})) def m (unpack F w) m.x`, "1.5"},
	}
	for _, tc := range cases {
		if got := runBytesCov(t, tc.src); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestBytesCovUnpackErrors(t *testing.T) {
	cases := []struct{ src, frag string }{
		// guards
		{`def P (refine BinarySpec [{value:127 type:'u8'} {name:'x' type:'u8'}]) unpack P (convert Bytes [1 2])`, "guard u8 != 127"},
		{`def P (refine BinarySpec [{value:5 type:'bits' size:4} {name:'y' type:'bits' size:4}]) unpack P (convert Bytes [31])`, "guard bits != 5"},
		{`def P (refine BinarySpec [{name:'x' type:'bits' size:4} {name:'p' type:'pad' size:4}]) unpack P (convert Bytes [81])`, "nonzero padding"},
		// short buffers
		{`def P (refine BinarySpec [{name:'x' type:'bits' size:4} {name:'p' type:'pad' size:4}]) unpack P (convert Bytes [])`, "too short"},
		{`def P (refine BinarySpec [{name:'x' type:'u16'}]) unpack P (convert Bytes [1])`, "too short"},
		{`def P (refine BinarySpec [{name:'b' type:'bytes' size:5}]) unpack P (convert Bytes [1 2])`, "too short"},
		{`def P (refine BinarySpec [{name:'x' type:'f64'}]) unpack P (convert Bytes [1 2])`, "too short"},
		// size / alignment / encoding
		{`def P (refine BinarySpec [{name:'b' type:'bytes' size:'len'}]) unpack P (convert Bytes [104 105])`, "not an earlier field"},
		{`def P (refine BinarySpec [{name:'x' type:'bits' size:4} {name:'b' type:'bytes' size:1}]) unpack P (convert Bytes [1 2])`, "not byte-aligned"},
		{`def P (refine BinarySpec [{name:'x' type:'bits' size:4}]) unpack P (convert Bytes [80])`, "trailing sub-byte bits"},
		{`def P (refine BinarySpec [{name:'s' type:'utf8'}]) unpack P (convert Bytes [255])`, "bad-encoding"},
		// u64 overflow of Integer
		{`def P (refine BinarySpec [{name:'x' type:'u64'}]) unpack P (convert Bytes [255 255 255 255 255 255 255 255])`, "exceeds Integer range"},
		// not a frame type
		{`def C class {a:1} unpack C (convert Bytes "x")`, "not a BinarySpec"},
		{`def C class {a:1} unpack-prefix C (convert Bytes "x")`, "not a BinarySpec"},
	}
	for _, tc := range cases {
		err := runBytesCovErr(t, tc.src)
		if err == nil || !strings.Contains(err.Error(), tc.frag) {
			t.Errorf("%s: error = %v, want fragment %q", tc.src, err, tc.frag)
		}
	}
}

func TestBytesCovUnpackPrefix(t *testing.T) {
	cases := []struct{ src, want string }{
		// a whole frame plus leftovers
		{`def P (refine BinarySpec [{name:'ver' type:'u8'} {name:'len' type:'u16'} {name:'body' type:'bytes' size:'len'}]) unpack-prefix P (convert Bytes [1 0 2 104 105 170])`, "{ok:Class/P{ver:1 len:2 body:Bytes<68 69>} rest:Bytes<aa>}"},
		// short buffers report how many more bytes are needed
		{`def P (refine BinarySpec [{name:'ver' type:'u8'} {name:'len' type:'u16'}]) unpack-prefix P (convert Bytes [0])`, "{need:2}"},
		{`def P (refine BinarySpec [{name:'x' type:'bits' size:8}]) unpack-prefix P (convert Bytes [])`, "{need:1}"},
		{`def P (refine BinarySpec [{name:'b' type:'bytes' size:5}]) unpack-prefix P (convert Bytes [1 2])`, "{need:3}"},
		{`def P (refine BinarySpec [{name:'x' type:'f64'}]) unpack-prefix P (convert Bytes [1 2])`, "{need:6}"},
		// an exact frame leaves empty rest
		{`def P (refine BinarySpec [{name:'x' type:'u8'}]) unpack-prefix P (convert Bytes [9])`, "{ok:Class/P{x:9} rest:Bytes<>}"},
	}
	for _, tc := range cases {
		if got := runBytesCov(t, tc.src); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// Frame instances stay first-class objects: field access, typeof, is.
func TestBytesCovFrameInstanceSurface(t *testing.T) {
	cases := []struct{ src, want string }{
		{`def H (refine BinarySpec [{value:1 type:'u8'} {name:'len' type:'u16'} {name:'body' type:'bytes' size:'len'}]) def p (make H {len:2 body:(convert Bytes "hi")}) p.len`, "2"},
		{`def H (refine BinarySpec [{name:'x' type:'u8'}]) typeof (make H {x:1})`, "H"},
		{`def H (refine BinarySpec [{name:'x' type:'u8'}]) (make H {x:1}) is H`, "true"},
		{`def H (refine BinarySpec [{name:'x' type:'u8'}]) 5 is H`, "false"},
	}
	for _, tc := range cases {
		if got := runBytesCov(t, tc.src); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// ---- second wave: branches the language surface cannot reach ----------------

// The Go bridge closures installed in init(): []byte <-> Bytes with
// copy-on-ingest and copy-on-export.
func TestBytesCovGoBridge(t *testing.T) {
	src := []byte("ab")
	v := core.FromNative(src)
	b, ok := asBytes(v)
	if !ok || string(b) != "ab" {
		t.Fatalf("FromNative([]byte) = %v", v)
	}
	src[0] = 'z' // the bridge must have copied
	b, _ = asBytes(v)
	if string(b) != "ab" {
		t.Error("FromNative must copy-on-ingest")
	}

	out, isBytes := core.ToNative(v).([]byte)
	if !isBytes || string(out) != "ab" {
		t.Fatalf("ToNative(Bytes) = %v", out)
	}
	out[0] = 'z' // the export must be a fresh copy
	b, _ = asBytes(v)
	if string(b) != "ab" {
		t.Error("ToNative must copy-on-export")
	}

	// A non-Bytes value declines the hook.
	if _, isBytes := core.ToNative(NewInteger(1)).([]byte); isBytes {
		t.Error("ToNative(Integer) must not produce []byte")
	}
}

// Handler-level error branches for arguments dispatch cannot deliver
// (type literals in the data slot).
func TestBytesCovHandlerErrorBranches(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	litB := NewTypeLiteral(TBytes)
	litS := NewTypeLiteral(TString)
	litL := NewTypeLiteral(TList)

	if _, herr := convertStringToBytes([]Value{litB, litS}, nil, nil, r); herr == nil {
		t.Error("convertStringToBytes(type literal) must error")
	}
	if _, herr := convertBytesToString([]Value{litS, litB}, nil, nil, r); herr == nil {
		t.Error("convertBytesToString(type literal) must error")
	}
	if _, herr := convertBytesToList([]Value{litL, litB}, nil, nil, r); herr == nil {
		t.Error("convertBytesToList(type literal) must error")
	}
	if _, herr := convertBytesToBytes([]Value{litB, litB}, nil, nil, r); herr == nil {
		t.Error("convertBytesToBytes(type literal) must error")
	}
	if _, herr := convertListToBytes([]Value{litB, litL}, nil, nil, r); herr == nil {
		t.Error("convertListToBytes(type literal) must error")
	}
	if _, herr := readBitSegments(litL, r, "w"); herr == nil {
		t.Error("readBitSegments(type literal) must error")
	}
	if _, ok := binarySpecLayout(NewInteger(1)); ok {
		t.Error("binarySpecLayout(Integer) must decline")
	}
	if _, ok := binaryInstanceLayout(NewInteger(1)); ok {
		t.Error("binaryInstanceLayout(Integer) must decline")
	}
	// A Bytes-typed value with a foreign payload is not Bytes data.
	if _, ok := asBytes(core.NewValueRaw(TBytes, &TimeoutInfo{})); ok {
		t.Error("asBytes(non-extension payload) must decline")
	}
	if _, ok := asBytes(core.NewExtension(TBytes, "not-bytes")); ok {
		t.Error("asBytes(extension with non-[]byte body) must decline")
	}
	if (bytesBehavior{}).Equal(NewInteger(1), newBytes([]byte("a"))) {
		t.Error("Integer must not equal Bytes")
	}
}

// packSeg's missing-field / wrong-type branches: `make` validates fields
// against the schema, so these are only reachable with a handcrafted map.
func TestBytesCovPackSegDirect(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	empty, merr := RequireConcreteMap(NewMap(NewOrderedMap()), "t")
	if merr != nil {
		t.Fatal(merr)
	}
	missing := []bitSeg{
		{name: "x", typ: "u8", width: 1},
		{name: "x", typ: "f32", width: 4},
		{name: "x", typ: "f64", width: 8},
		{name: "x", typ: "bytes"},
		{name: "x", typ: "utf8"},
		{name: "x", typ: "bits", hasSize: true, sizeLit: 4},
	}
	for _, seg := range missing {
		w := &bitWriter{}
		if perr := packSeg(w, seg, empty, r, "convert"); perr == nil ||
			!strings.Contains(perr.Error(), "not provided") {
			t.Errorf("packSeg(%s, empty fields) err = %v, want 'not provided'", seg.typ, perr)
		}
	}

	// Wrong field types.
	om := NewOrderedMap()
	om.Set("x", NewInteger(1))
	badFields, merr := RequireConcreteMap(NewMap(om), "t")
	if merr != nil {
		t.Fatal(merr)
	}
	bytesSegSpec := bitSeg{name: "x", typ: "bytes"}
	if perr := packSeg(&bitWriter{}, bytesSegSpec, badFields, r, "convert"); perr == nil ||
		!strings.Contains(perr.Error(), "is not Bytes") {
		t.Errorf("packSeg(bytes, Integer field) err = %v", perr)
	}
	om2 := NewOrderedMap()
	om2.Set("x", newBytes([]byte("b")))
	badFields2, merr := RequireConcreteMap(NewMap(om2), "t")
	if merr != nil {
		t.Fatal(merr)
	}
	utf8SegSpec := bitSeg{name: "x", typ: "utf8"}
	if perr := packSeg(&bitWriter{}, utf8SegSpec, badFields2, r, "convert"); perr == nil ||
		!strings.Contains(perr.Error(), "is not a String") {
		t.Errorf("packSeg(utf8, Bytes field) err = %v", perr)
	}
}

// The binding decode mode (decodeSegs bind=true) installs fields as defs and
// reports short buffers as hard errors rather than needMore.
func TestBytesCovDecodeBind(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mkSpec := func(kvs ...func(*OrderedMap)) Value {
		var elems []Value
		for _, fill := range kvs {
			om := NewOrderedMap()
			fill(om)
			elems = append(elems, NewMap(om))
		}
		return NewList(elems)
	}
	intSpec := mkSpec(func(om *OrderedMap) {
		om.Set("name", NewString("bx"))
		om.Set("type", NewString("u8"))
	})
	segs, serr := readBitSegments(intSpec, r, "unpack")
	if serr != nil {
		t.Fatal(serr)
	}
	out, rest, derr := decodeSegs([]byte{5, 9}, segs, r, "unpack", true)
	if derr != nil {
		t.Fatalf("bind decode: %v", derr)
	}
	if out.Len() != 0 {
		t.Errorf("bind decode must install defs, not collect: %v", out)
	}
	if len(rest) != 1 || rest[0] != 9 {
		t.Errorf("bind decode rest = %v, want [9]", rest)
	}
	if v, ok := r.Defs.Top("bx"); !ok {
		t.Error("bind decode must install the field as a def")
	} else if n, aerr := AsInteger(v); aerr != nil || n != 5 {
		t.Errorf("bound field = %v, want 5", v)
	}

	// Short buffer, binding mode: a hard no_match error (fixed-int path).
	if _, _, derr = decodeSegs(nil, segs, r, "unpack", true); derr == nil ||
		!strings.Contains(derr.Error(), "buffer too short") {
		t.Errorf("bind short int err = %v, want 'buffer too short'", derr)
	}

	// Same for the bit-level path.
	bitsSpec := mkSpec(func(om *OrderedMap) {
		om.Set("name", NewString("bb"))
		om.Set("type", NewString("bits"))
		om.Set("size", NewInteger(8))
	})
	bitSegs, serr := readBitSegments(bitsSpec, r, "unpack")
	if serr != nil {
		t.Fatal(serr)
	}
	if _, _, derr = decodeSegs(nil, bitSegs, r, "unpack", true); derr == nil ||
		!strings.Contains(derr.Error(), "buffer too short") {
		t.Errorf("bind short bits err = %v, want 'buffer too short'", derr)
	}
}

func TestBytesCovShortErrHelpers(t *testing.T) {
	nm := needMore{n: 2}
	if e := nm.Error(); !strings.Contains(e, "need more bytes") {
		t.Errorf("needMore.Error() = %q", e)
	}
	// Non-binding mode wraps the shortfall as needMore, clamped to >= 1.
	if ne, ok := shortErr("w", false).(needMore); !ok || ne.n != 1 {
		t.Errorf("shortErr(false) = %v", ne)
	}
	if ne, ok := shortErr2("w", false, 0).(needMore); !ok || ne.n != 1 {
		t.Errorf("shortErr2(false, 0) = %v, want need 1", ne)
	}
	if ne, ok := shortErr2("w", false, 3).(needMore); !ok || ne.n != 3 {
		t.Errorf("shortErr2(false, 3) = %v, want need 3", ne)
	}
	// Binding mode is a hard error, not a needMore.
	if _, ok := shortErr("w", true).(needMore); ok {
		t.Error("shortErr(true) must not be needMore")
	}
	if _, ok := shortErr2("w", true, 3).(needMore); ok {
		t.Error("shortErr2(true) must not be needMore")
	}
}

// A few remaining wire-format corners through the surface.
func TestBytesCovWireCorners(t *testing.T) {
	cases := []struct{ src, want string }{
		// signed positive byte: no sign extension
		{`def P (refine BinarySpec [{name:'x' type:'i8'}]) def m (unpack P (convert Bytes [5])) m.x`, "5"},
		// a full 64-bit bits field packs without truncation
		{`def P (refine BinarySpec [{name:'x' type:'bits' size:64}]) convert Bytes (make P {x:1})`, "Bytes<00 00 00 00 00 00 00 01>"},
		// slice with both bounds beyond the data clamps to empty
		{`convert String (slice 5 9 (convert Bytes "hi"))`, "''"},
	}
	for _, tc := range cases {
		if got := runBytesCov(t, tc.src); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}
