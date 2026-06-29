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
			"grammar is shared: `name:type[/suffix]*(size)` with types u8..u64/" +
			"i8..i64/f32/f64/bits/bytes/utf8/pad, slash suffixes be(default)/le/" +
			"signed/unsigned, and a paren `(size)` (int or a previously-bound " +
			"name) for bytes/utf8/bits/pad. Hex/binary byte constants are the " +
			"`+hb/…/` and `+bb/…/` minilang kinds (import \"aql:minilang\").",
		Examples: []string{
			`import "aql:minilang"  unpack-prefix (+hb/0100026869/) [op:u8 len:u16 body:bytes(len)]   # {ok:{op:1 len:2 body:Bytes<68 69>} rest:Bytes<>}`,
			`unpack-prefix (convert Bytes [0]) [op:u8 len:u16]   # {need: 2}`,
		},
	})
}
