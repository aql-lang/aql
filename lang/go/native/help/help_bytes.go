package help

// Bytes (Scalar/Bytes) is exposed through SIGNATURE OVERLOADS of existing
// words rather than dedicated words: `convert` handles String/List<->Bytes
// and `convert Bytes <bytes>` compacts; `slice` takes sub-ranges; `add`
// concatenates; `make Bytes [spec]` packs a bit-syntax frame; `unpack`
// decodes one. Generic `size`/`cmp`/`eq`/`sort` work via the type's
// behaviors. Only the streaming framer `unpack-prefix` is its own word.
// See design/go-modules/BYTES.10.md. Heavier binary ops (hashes, HMAC,
// hex/base encodings, secure random, UUID) live in aql:bin-util.
func init() {
	register(&Entry{
		Word:    "unpack-prefix",
		Summary: "Decode a leading bit-syntax frame from Bytes, returning the rest.",
		Description: "`unpack-prefix b [spec]` matches as much of the Bytes `b` as " +
			"the segment spec needs and returns `{ok: <fields> rest: <Bytes>}` with " +
			"the bound fields and the leftover bytes — or `{need: n}` when the " +
			"buffer is too short. The streaming-framer counterpart to `unpack` " +
			"(which decodes a whole frame and binds names into scope). Segment " +
			"grammar is shared: `name:type[/suffix]*` with types u8..u64/i8..i64/" +
			"f32/f64/bits/bytes/utf8/pad, suffixes be(default)/le/signed/unsigned " +
			"or a size (int or a previously-bound name) for bytes/utf8/bits/pad.",
		Examples: []string{
			`unpack-prefix 0x"0100056869" [op:u8 len:u16 body:bytes/len]   # {ok:{op:1 len:5 body:Bytes<68 69>} rest:Bytes<>}`,
			`unpack-prefix 0x"00" [op:u8 len:u16]                          # {need: 2}`,
		},
	})
}
