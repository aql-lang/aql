package native

import (
	"bytes"
	"fmt"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

// io_encoding.go implements the text-encoding leg of read/write's `enc`
// option: utf8 (the default, a pass-through), utf16le / utf16be (BOM
// emitted on write, a matching leading BOM stripped on read), and latin1
// (ISO 8859-1, strict — a rune outside Latin-1 is a write error, never a
// silent substitution). `bytes` / `binary` never reach these helpers —
// the binary read path (io_binary.go tryBinaryRead) and the Bytes write
// signatures intercept first — so an unrecognised enc arriving here is a
// hard error rather than the historical silent no-op.
//
// Policy (deliberate, documented in REFERENCE.md): encoding is EXPLICIT
// ONLY. An unspecified enc means utf8; there is no BOM sniffing. A BOM of
// the CONTRARY endianness under a declared utf16 enc is not a marker —
// it decodes as an ordinary character. Malformed input under utf16
// (odd-length tail, lone surrogate) decodes to U+FFFD replacement runes,
// mirroring Go's own string conversion posture.

// utf16leBOM / utf16beBOM are the byte-order marks the utf16 encodings
// emit on write and strip on read.
var (
	utf16leBOM = []byte{0xFF, 0xFE}
	utf16beBOM = []byte{0xFE, 0xFF}
)

// encErrValid is the valid-set hint appended to unknown-enc errors.
const encErrValid = "valid encodings: utf8, bytes, utf16le, utf16be, latin1"

// utf16Codec returns the x/text codec for the given endianness. IgnoreBOM
// keeps the transformer BOM-agnostic — BOM handling is ours (strip on
// read, emit on write) so the policy stays in one visible place.
func utf16Codec(bigEndian bool) encoding.Encoding {
	endian := unicode.LittleEndian
	if bigEndian {
		endian = unicode.BigEndian
	}
	return unicode.UTF16(endian, unicode.IgnoreBOM)
}

// decodeEnc converts raw file bytes into a Go (UTF-8) string per enc.
// utf16 strips one leading BOM of the MATCHING endianness; an unknown enc
// errors. The x/text UTF-16 decoder and the ISO 8859-1 decoder are TOTAL —
// malformed sequences become U+FFFD and every Latin-1 byte maps to a rune,
// so neither returns an error (verified empirically; the blank error
// assignments below are the provably-dead-guard idiom, see
// design/TEST-SEAMS.10.md and MemFileOps.ResolvePath).
func decodeEnc(data []byte, enc string) (string, error) {
	switch enc {
	case "", "utf8":
		return string(data), nil
	case "utf16le", "utf16be":
		bom := utf16leBOM
		big := false
		if enc == "utf16be" {
			bom = utf16beBOM
			big = true
		}
		data = bytes.TrimPrefix(data, bom)
		out, _ := utf16Codec(big).NewDecoder().Bytes(data)
		return string(out), nil
	case "latin1":
		out, _ := charmap.ISO8859_1.NewDecoder().Bytes(data)
		return string(out), nil
	default:
		return "", fmt.Errorf("unknown encoding %q (%s)", enc, encErrValid)
	}
}

// encodeEnc converts a Go string into file bytes per enc. utf16 output is
// BOM-prefixed (its encoder is total — an invalid UTF-8 input byte becomes
// U+FFFD, so the blank error assignment is the same dead-guard idiom as
// decodeEnc's); latin1 is strict — an unmappable rune errors; an unknown
// enc errors.
func encodeEnc(content, enc string) ([]byte, error) {
	switch enc {
	case "", "utf8":
		return []byte(content), nil
	case "utf16le", "utf16be":
		bom := utf16leBOM
		big := false
		if enc == "utf16be" {
			bom = utf16beBOM
			big = true
		}
		out, _ := utf16Codec(big).NewEncoder().Bytes([]byte(content))
		return append(append([]byte{}, bom...), out...), nil
	case "latin1":
		out, err := charmap.ISO8859_1.NewEncoder().Bytes([]byte(content))
		if err != nil {
			return nil, fmt.Errorf("encode latin1: %w (the text has a character outside ISO 8859-1)", err)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unknown encoding %q (%s)", enc, encErrValid)
	}
}
