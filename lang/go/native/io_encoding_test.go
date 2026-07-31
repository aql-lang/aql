package native

import (
	"bytes"
	"strings"
	"testing"
)

func TestDecodeEnc(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		enc  string
		want string
	}{
		{"utf8 default", []byte("hi"), "", "hi"},
		{"utf8 named", []byte("hi"), "utf8", "hi"},
		{"utf16le with BOM", []byte{0xFF, 0xFE, 0x68, 0x00, 0x69, 0x00}, "utf16le", "hi"},
		{"utf16le without BOM", []byte{0x68, 0x00, 0x69, 0x00}, "utf16le", "hi"},
		{"utf16be with BOM", []byte{0xFE, 0xFF, 0x00, 0x68, 0x00, 0x69}, "utf16be", "hi"},
		{"utf16be without BOM", []byte{0x00, 0x68, 0x00, 0x69}, "utf16be", "hi"},
		// A CONTRARY BOM is not a marker: under utf16le the BE BOM bytes
		// FE FF are the LE code unit 0xFFFE (a noncharacter, passed through
		// verbatim) — the point is it is NOT stripped.
		{"contrary BOM decodes as data", []byte{0xFE, 0xFF, 0x68, 0x00}, "utf16le", "\ufffeh"},
		{"latin1 high bytes", []byte{0x68, 0xE9}, "latin1", "hé"},
		// Malformed utf16 tails decode to replacement runes, never error.
		{"utf16le odd tail", []byte{0x68, 0x00, 0x69}, "utf16le", "h�"},
	}
	for _, c := range cases {
		got, err := decodeEnc(c.data, c.enc)
		if err != nil {
			t.Errorf("%s: unexpected error %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: decodeEnc = %q, want %q", c.name, got, c.want)
		}
	}
	if _, err := decodeEnc([]byte("x"), "bogus"); err == nil || !strings.Contains(err.Error(), "unknown encoding") {
		t.Errorf("unknown enc decode: err = %v, want unknown-encoding", err)
	}
}

func TestEncodeEnc(t *testing.T) {
	// utf8 pass-through.
	if got, err := encodeEnc("hi", "utf8"); err != nil || string(got) != "hi" {
		t.Errorf("utf8 encode = %q, %v", got, err)
	}
	// utf16le: BOM + little-endian units.
	got, err := encodeEnc("hi", "utf16le")
	if err != nil || !bytes.Equal(got, []byte{0xFF, 0xFE, 0x68, 0x00, 0x69, 0x00}) {
		t.Errorf("utf16le encode = % x, %v", got, err)
	}
	// utf16be: BOM + big-endian units.
	got, err = encodeEnc("hi", "utf16be")
	if err != nil || !bytes.Equal(got, []byte{0xFE, 0xFF, 0x00, 0x68, 0x00, 0x69}) {
		t.Errorf("utf16be encode = % x, %v", got, err)
	}
	// latin1 maps into single bytes.
	got, err = encodeEnc("hé", "latin1")
	if err != nil || !bytes.Equal(got, []byte{0x68, 0xE9}) {
		t.Errorf("latin1 encode = % x, %v", got, err)
	}
	// latin1 is STRICT: a rune outside ISO 8859-1 errors, never substitutes.
	if _, err := encodeEnc("€", "latin1"); err == nil || !strings.Contains(err.Error(), "outside ISO 8859-1") {
		t.Errorf("latin1 euro encode: err = %v, want outside-ISO-8859-1", err)
	}
	// Unknown enc errors.
	if _, err := encodeEnc("x", "bogus"); err == nil || !strings.Contains(err.Error(), "unknown encoding") {
		t.Errorf("unknown enc encode: err = %v, want unknown-encoding", err)
	}
}

// TestEncRoundTripThroughWords drives the enc option end-to-end through the
// read/write words against the mem FS, including the decode-before-applyNL
// ordering pin: a utf16 CRLF is `\r\x00\n\x00` on the wire, so normalizing
// before decoding would corrupt the stream.
func TestEncRoundTripThroughWords(t *testing.T) {
	r, mem := ioFSReg(t)

	// Seed a utf16le file containing "a\r\nb" (BOM + LE units).
	crlf := []byte{0xFF, 0xFE, 'a', 0x00, '\r', 0x00, '\n', 0x00, 'b', 0x00}
	if err := mem.WriteFile("crlf16.txt", crlf, 0644); err != nil {
		t.Fatal(err)
	}
	res := runBoru(t, r, []Value{
		NewWord("read"), pathV("crlf16.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("enc", NewString("utf16le")) }),
	})
	if res[0].String() != "'a\nb'" {
		t.Errorf("utf16 CRLF read = %v, want 'a\\nb' (decode must precede applyNL)", res[0])
	}

	// utf16be write emits the BE BOM.
	runBoru(t, r, []Value{
		NewWord("write"), pathV("be.txt"), NewString("hi"),
		wrapMap(func(om *OrderedMap) { om.Set("enc", NewString("utf16be")) }),
	})
	if got, _ := mem.ReadFile("be.txt"); !bytes.Equal(got, []byte{0xFE, 0xFF, 0x00, 0x68, 0x00, 0x69}) {
		t.Errorf("utf16be write stored % x", got)
	}

	// Append merges at the text level: exactly one leading BOM.
	runBoru(t, r, []Value{
		NewWord("write"), pathV("ap.txt"), NewString("ab"),
		wrapMap(func(om *OrderedMap) { om.Set("enc", NewString("utf16le")) }),
	})
	runBoru(t, r, []Value{
		NewWord("write"), pathV("ap.txt"), NewString("cd"),
		wrapMap(func(om *OrderedMap) { om.Set("enc", NewString("utf16le")); om.Set("mode", NewString("append")) }),
	})
	got, _ := mem.ReadFile("ap.txt")
	if bytes.Count(got, []byte{0xFF, 0xFE}) != 1 {
		t.Errorf("utf16 append produced an interior BOM: % x", got)
	}

	// Unknown enc on write errors; on an append over an EXISTING file the
	// append-decode guard fires first (same taxonomy, distinct branch).
	if err := runBoruError(t, r, []Value{
		NewWord("write"), pathV("new.txt"), NewString("x"),
		wrapMap(func(om *OrderedMap) { om.Set("enc", NewString("bogus")) }),
	}); err == nil || !strings.Contains(err.Error(), "unknown encoding") {
		t.Errorf("bogus enc write: err = %v", err)
	}
	if err := runBoruError(t, r, []Value{
		NewWord("write"), pathV("ap.txt"), NewString("x"),
		wrapMap(func(om *OrderedMap) { om.Set("enc", NewString("bogus")); om.Set("mode", NewString("append")) }),
	}); err == nil || !strings.Contains(err.Error(), "append") {
		t.Errorf("bogus enc append: err = %v, want the append-decode guard", err)
	}

	// Unknown enc on read errors.
	if err := runBoruError(t, r, []Value{
		NewWord("read"), pathV("ap.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("enc", NewString("bogus")) }),
	}); err == nil || !strings.Contains(err.Error(), "unknown encoding") {
		t.Errorf("bogus enc read: err = %v", err)
	}
}
