package help

// Bytes (Scalar/Bytes) is a plain binary Scalar. Its value operations are
// SIGNATURE OVERLOADS of existing words: `convert` handles String/List<->Bytes
// and `convert Bytes <bytes>` compacts; `slice` takes sub-ranges; `add`
// concatenates; generic `size`/`cmp`/`eq`/`sort` work via the type's behaviors.
// Binary FRAME layouts are a separate concern: a frame is a TYPE you define by
// refining `BinarySpec` (BinarySpec : Binary :: a class type : its instance) —
// `def Header (refine BinarySpec [layout])`. `make Header {fields}` builds a
// field-accessible Binary INSTANCE (like a class instance), `convert Bytes
// <inst>` serialises it to wire Bytes, and `unpack`/`unpack-prefix` decode wire
// Bytes back into an instance (ADR-007 — the layout is plain Node data on the
// type, not a parsed spec). See design/go-modules/BYTES.10.md. Heavier binary
// ops (hashes, HMAC, hex/base encodings, secure random, UUID) live in
// boru:bin-util.
func init() {
	register(&Entry{
		Word:    "unpack-prefix",
		Summary: "Decode a leading frame from Bytes against a frame type, returning the rest.",
		Description: "`unpack-prefix <BinarySpec> b` matches as much of the Bytes `b` as " +
			"the frame type's layout needs and returns `{ok: <Binary> rest: <Bytes>}` " +
			"with the decoded Binary instance and the leftover bytes — or `{need: n}` " +
			"when the buffer is too short. The streaming-framer counterpart to `unpack` " +
			"(which decodes a whole frame to a Binary instance). The frame type is " +
			"defined with `def P (refine BinarySpec [layout])`, where the layout is a " +
			"`List` of segment `Map`s — keys `name`|`value`, `type` (u8..u64/i8..i64/" +
			"f32/f64/bits/bytes/utf8/pad), `endian` (be/le), `signed`, `size` (Integer " +
			"or a field-name String). Hex/binary byte constants are the `+hb/…/` / " +
			"`+bb/…/` minilang kinds (import \"boru:minilang\").",
		Examples: []string{
			`def Msg (refine BinarySpec [{name:'op' type:'u8'} {name:'len' type:'u16'} {name:'body' type:'bytes' size:'len'}])  unpack-prefix Msg (convert Bytes [1 0 2 104 105])   # {ok:Class/Msg{op:1 len:2 body:Bytes<68 69>} rest:Bytes<>}`,
			`def Hdr (refine BinarySpec [{name:'op' type:'u8'} {name:'len' type:'u16'}]) unpack-prefix Hdr (convert Bytes [0]) # {need: 2}`,
		},
	})
}
