package modules

import (
	"testing"

	"github.com/aql-lang/aql/lang/go/native"
)

// binRegistry returns a registry with the aql:bin module installed.
func binRegistry(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	desc, err := BuildBinaryModule(r)
	if err != nil {
		t.Fatal(err)
	}
	for name, exportMap := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exportMap))
	}
	return r
}

func TestBinResolve(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	desc, err := Resolve("bin", r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	binExport, ok := desc.Exports["bin"]
	if !ok {
		t.Fatal("expected 'bin' export in module descriptor")
	}
	// Spot-check a few names.
	for _, name := range []string{"popcount", "clz", "ctz", "rotl", "rotr", "test", "set", "clear", "toggle", "mask", "extract", "insert", "reverse", "swap", "bitlen", "parity"} {
		if _, ok := binExport.Get(name); !ok {
			t.Errorf("missing 'bin.%s' export", name)
		}
	}
}

func runBin(t *testing.T, tokens []native.Value) []native.Value {
	t.Helper()
	r := binRegistry(t)
	e := native.New(r)
	out, err := e.Run(tokens)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return out
}

// dotChain emits the `bin.<name>` access pattern: equivalent to
// `bin get <name>/q`.
func dotChain(name string) []native.Value {
	return []native.Value{
		native.NewWord("bin"),
		native.NewWord("get"),
		native.NewAtom(name),
	}
}

func TestBinPopcount(t *testing.T) {
	cases := []struct {
		x    int64
		want int64
	}{
		{0, 0},
		{1, 1},
		{0xff, 8},
		{-1, 64},
		{0xaa55, 8},
	}
	for _, c := range cases {
		tokens := append([]native.Value{native.NewInteger(c.x)}, dotChain("popcount")...)
		got := runBin(t, tokens)
		if len(got) != 1 {
			t.Fatalf("popcount %d: expected 1 result, got %d", c.x, len(got))
		}
		n, _ := native.AsInteger(got[0])
		if n != c.want {
			t.Errorf("popcount %d = %d, want %d", c.x, n, c.want)
		}
	}
}

func TestBinClzCtz(t *testing.T) {
	cases := []struct {
		x      int64
		clz    int64
		ctz    int64
		bitlen int64
		parity bool
	}{
		{0, 64, 64, 0, false},
		{1, 63, 0, 1, true},
		{8, 60, 3, 4, true},
		{0xff, 56, 0, 8, false},
		{-1, 0, 0, 64, false},
	}
	for _, c := range cases {
		r := binRegistry(t)
		e := native.New(r)
		out, err := e.Run(append([]native.Value{native.NewInteger(c.x)}, dotChain("clz")...))
		if err != nil {
			t.Fatalf("clz %d: %v", c.x, err)
		}
		if n, _ := native.AsInteger(out[0]); n != c.clz {
			t.Errorf("clz %d = %d, want %d", c.x, n, c.clz)
		}

		out, err = e.Run(append([]native.Value{native.NewInteger(c.x)}, dotChain("ctz")...))
		if err != nil {
			t.Fatalf("ctz %d: %v", c.x, err)
		}
		if n, _ := native.AsInteger(out[0]); n != c.ctz {
			t.Errorf("ctz %d = %d, want %d", c.x, n, c.ctz)
		}

		out, err = e.Run(append([]native.Value{native.NewInteger(c.x)}, dotChain("bitlen")...))
		if err != nil {
			t.Fatalf("bitlen %d: %v", c.x, err)
		}
		if n, _ := native.AsInteger(out[0]); n != c.bitlen {
			t.Errorf("bitlen %d = %d, want %d", c.x, n, c.bitlen)
		}

		out, err = e.Run(append([]native.Value{native.NewInteger(c.x)}, dotChain("parity")...))
		if err != nil {
			t.Fatalf("parity %d: %v", c.x, err)
		}
		if b, _ := native.AsBoolean(out[0]); b != c.parity {
			t.Errorf("parity %d = %v, want %v", c.x, b, c.parity)
		}
	}
}

func TestBinRotates(t *testing.T) {
	r := binRegistry(t)
	e := native.New(r)
	// 1 bin.rotl 1 → 2
	out, err := e.Run(append([]native.Value{native.NewInteger(1)}, append(dotChain("rotl"), native.NewInteger(1))...))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := native.AsInteger(out[0]); n != 2 {
		t.Errorf("1 rotl 1 = %d, want 2", n)
	}
	// 1 bin.rotr 1 → high bit set: -9223372036854775808
	e2 := native.New(r)
	out, err = e2.Run(append([]native.Value{native.NewInteger(1)}, append(dotChain("rotr"), native.NewInteger(1))...))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := native.AsInteger(out[0]); n != -1<<63 {
		t.Errorf("1 rotr 1 = %d, want %d", n, int64(-1<<63))
	}
	// Rotation by 64 is the identity (mod 64).
	e3 := native.New(r)
	out, err = e3.Run(append([]native.Value{native.NewInteger(0xdeadbeef)}, append(dotChain("rotl"), native.NewInteger(64))...))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := native.AsInteger(out[0]); n != 0xdeadbeef {
		t.Errorf("0xdeadbeef rotl 64 = %d, want 0xdeadbeef", n)
	}
}

func TestBinSingleBit(t *testing.T) {
	r := binRegistry(t)
	// 0xa5 (= 10100101) bin.test 0 → true
	e := native.New(r)
	out, err := e.Run(append([]native.Value{native.NewInteger(0xa5)}, append(dotChain("test"), native.NewInteger(0))...))
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := native.AsBoolean(out[0]); !b {
		t.Error("0xa5 test 0 should be true")
	}
	// 0xa5 bin.test 1 → false
	e2 := native.New(r)
	out, err = e2.Run(append([]native.Value{native.NewInteger(0xa5)}, append(dotChain("test"), native.NewInteger(1))...))
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := native.AsBoolean(out[0]); b {
		t.Error("0xa5 test 1 should be false")
	}
	// 0 bin.set 3 → 8
	e3 := native.New(r)
	out, err = e3.Run(append([]native.Value{native.NewInteger(0)}, append(dotChain("set"), native.NewInteger(3))...))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := native.AsInteger(out[0]); n != 8 {
		t.Errorf("0 set 3 = %d, want 8", n)
	}
	// 15 bin.clear 0 → 14
	e4 := native.New(r)
	out, err = e4.Run(append([]native.Value{native.NewInteger(15)}, append(dotChain("clear"), native.NewInteger(0))...))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := native.AsInteger(out[0]); n != 14 {
		t.Errorf("15 clear 0 = %d, want 14", n)
	}
	// 0 bin.toggle 5 → 32
	e5 := native.New(r)
	out, err = e5.Run(append([]native.Value{native.NewInteger(0)}, append(dotChain("toggle"), native.NewInteger(5))...))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := native.AsInteger(out[0]); n != 32 {
		t.Errorf("0 toggle 5 = %d, want 32", n)
	}
}

func TestBinMaskReverseSwap(t *testing.T) {
	r := binRegistry(t)
	// bin.mask 8 → 255
	e := native.New(r)
	out, err := e.Run(append(dotChain("mask"), native.NewInteger(8)))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := native.AsInteger(out[0]); n != 255 {
		t.Errorf("mask 8 = %d, want 255", n)
	}
	// bin.mask 0 → 0
	e2 := native.New(r)
	out, err = e2.Run(append(dotChain("mask"), native.NewInteger(0)))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := native.AsInteger(out[0]); n != 0 {
		t.Errorf("mask 0 = %d, want 0", n)
	}
	// bin.mask 64 → -1
	e3 := native.New(r)
	out, err = e3.Run(append(dotChain("mask"), native.NewInteger(64)))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := native.AsInteger(out[0]); n != -1 {
		t.Errorf("mask 64 = %d, want -1", n)
	}
	// 1 bin.reverse → high bit set: 1 << 63
	e4 := native.New(r)
	out, err = e4.Run(append([]native.Value{native.NewInteger(1)}, dotChain("reverse")...))
	if err != nil {
		t.Fatal(err)
	}
	want := int64(-1) << 63 // = -9223372036854775808
	if n, _ := native.AsInteger(out[0]); n != want {
		t.Errorf("1 reverse = %d, want %d", n, want)
	}
	// 0x0102030405060708 bin.swap → 0x0807060504030201
	e5 := native.New(r)
	out, err = e5.Run(append([]native.Value{native.NewInteger(0x0102030405060708)}, dotChain("swap")...))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := native.AsInteger(out[0]); n != 0x0807060504030201 {
		t.Errorf("0x0102030405060708 swap = %x, want 0x0807060504030201", n)
	}
}

func TestBinExtractInsert(t *testing.T) {
	r := binRegistry(t)
	// 0xabcd bin.extract 4 12 → 0xbc (bits [4, 12))
	e := native.New(r)
	out, err := e.Run([]native.Value{
		native.NewInteger(0xabcd), native.NewWord("bin"),
		native.NewWord("get"), native.NewAtom("extract"),
		native.NewInteger(4), native.NewInteger(12),
	})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := native.AsInteger(out[0]); n != 0xbc {
		t.Errorf("0xabcd extract 4 12 = %x, want bc", n)
	}
	// 0xabcd bin.insert 4 12 0x00 → low 4 bits unchanged (0xd), bits [4,12) zeroed
	// 0xabcd = 1010 1011 1100 1101 → clear bits 4..11 → 1010 0000 0000 1101 = 0xa00d
	e2 := native.New(r)
	out, err = e2.Run([]native.Value{
		native.NewInteger(0xabcd), native.NewWord("bin"),
		native.NewWord("get"), native.NewAtom("insert"),
		native.NewInteger(4), native.NewInteger(12), native.NewInteger(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := native.AsInteger(out[0]); n != 0xa00d {
		t.Errorf("0xabcd insert 4 12 0 = %x, want a00d", n)
	}
}

func TestBinSingleBitRangeError(t *testing.T) {
	r := binRegistry(t)
	e := native.New(r)
	_, err := e.Run([]native.Value{
		native.NewInteger(0), native.NewWord("bin"),
		native.NewWord("get"), native.NewAtom("set"),
		native.NewInteger(64),
	})
	if err == nil {
		t.Fatal("expected range error for set 64, got nil")
	}
}

// --- §9.8 ord / chr ---

func TestBinOrd(t *testing.T) {
	cases := []struct {
		s    string
		want int64
	}{
		{"A", 65},
		{"a", 97},
		{"0", 48},
		{"~", 126},
		{"€", 0x20AC}, // multi-byte rune
		{"abc", 97},   // first rune only
	}
	for _, c := range cases {
		out := runBin(t, append([]native.Value{native.NewString(c.s)}, dotChain("ord")...))
		if len(out) != 1 {
			t.Fatalf("ord %q: expected 1 result, got %d", c.s, len(out))
		}
		if n, _ := native.AsInteger(out[0]); n != c.want {
			t.Errorf("ord %q = %d, want %d", c.s, n, c.want)
		}
	}
}

func TestBinOrdEmptyErrors(t *testing.T) {
	r := binRegistry(t)
	e := native.New(r)
	_, err := e.Run(append([]native.Value{native.NewString("")}, dotChain("ord")...))
	if err == nil {
		t.Fatal("expected an error for ord of the empty string, got nil")
	}
}

func TestBinChr(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{65, "A"},
		{97, "a"},
		{48, "0"},
		{0x20AC, "€"},
	}
	for _, c := range cases {
		out := runBin(t, append([]native.Value{native.NewInteger(c.n)}, dotChain("chr")...))
		if len(out) != 1 {
			t.Fatalf("chr %d: expected 1 result, got %d", c.n, len(out))
		}
		if s, _ := native.AsString(out[0]); s != c.want {
			t.Errorf("chr %d = %q, want %q", c.n, s, c.want)
		}
	}
}

func TestBinChrOutOfRangeErrors(t *testing.T) {
	r := binRegistry(t)
	for _, n := range []int64{-1, 0x110000} {
		e := native.New(r)
		_, err := e.Run(append([]native.Value{native.NewInteger(n)}, dotChain("chr")...))
		if err == nil {
			t.Errorf("expected a range error for chr %d, got nil", n)
		}
	}
}

func TestBinOrdChrRoundTrip(t *testing.T) {
	for n := int64(32); n < 127; n++ {
		// chr n then ord → n
		toks := append([]native.Value{native.NewInteger(n)}, dotChain("chr")...)
		toks = append(toks, dotChain("ord")...)
		out := runBin(t, toks)
		if got, _ := native.AsInteger(out[0]); got != n {
			t.Errorf("ord(chr(%d)) = %d, want %d", n, got, n)
		}
	}
}

// --- §9.9 FNV-1a hashes ---

func TestBinFnv32(t *testing.T) {
	// Known FNV-1a/32 values.
	cases := []struct {
		s    string
		want int64
	}{
		{"", 2166136261},
		{"hello", 1335831723},
		{"a", 0xe40c292c},
	}
	for _, c := range cases {
		out := runBin(t, append([]native.Value{native.NewString(c.s)}, dotChain("fnv32")...))
		if n, _ := native.AsInteger(out[0]); n != c.want {
			t.Errorf("fnv32 %q = %d, want %d", c.s, n, c.want)
		}
	}
}

func TestBinFnv64NonNegativeAndStable(t *testing.T) {
	seen := map[int64]string{}
	for _, s := range []string{"", "a", "b", "hello", "world", "aql:bin"} {
		out := runBin(t, append([]native.Value{native.NewString(s)}, dotChain("fnv64")...))
		n, _ := native.AsInteger(out[0])
		if n < 0 {
			t.Errorf("fnv64 %q = %d, want non-negative (usable as hash mod m)", s, n)
		}
		// Re-hashing the same input is stable.
		out2 := runBin(t, append([]native.Value{native.NewString(s)}, dotChain("fnv64")...))
		if n2, _ := native.AsInteger(out2[0]); n2 != n {
			t.Errorf("fnv64 %q not stable: %d vs %d", s, n, n2)
		}
		if prev, ok := seen[n]; ok && prev != s {
			t.Errorf("fnv64 collision between %q and %q (both %d)", prev, s, n)
		}
		seen[n] = s
	}
}
