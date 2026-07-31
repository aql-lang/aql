# `boru:codec` Representation Module

**Status:** Discovery draft with boundary-review gate  
**Module:** `boru:codec`

## 1. Purpose

boru already has ParseLang and built-in facilities for structured formats such as JSON and CSV. A new codec module must not duplicate that work.

The proposed boundary is:

- **ParseLang:** structured languages and documents.
- **BinUtil:** binary values, slicing, concatenation, bit operations and binary layout manipulation.
- **Codec:** representation and transport encodings that transform one representation into another.

The module should remain small if these boundaries are working.

## 2. Candidate scope

- Base64 and URL-safe Base64.
- Hexadecimal encoding.
- Percent/URL component encoding where not already owned by networking.
- PEM envelope encoding/decoding.
- DER and ASN.1 interoperability where a typed representation is established.
- Character-set conversion, particularly UTF families.
- Byte-order-mark inspection and normalization.
- Text/binary representation transforms required by crypto and protocols.

## 3. BinUtil boundary

Integer packing, field offsets, bit fields and declarative binary layouts overlap strongly with BinUtil. The default disposition is that binary layout construction belongs in BinUtil, while Codec may consume BinUtil values to perform named representation transforms.

Before implementation:

1. inventory existing BinUtil exports;
2. identify exact missing transforms;
3. avoid introducing two ways to pack integers or choose endianness;
4. retain a single canonical binary carrier;
5. decide where UTF code-unit packing belongs.

If the resulting codec surface is only a handful of transforms, that is acceptable.

## 4. ParseLang boundary

JSON, YAML, CSV and other structured formats remain ParseLang responsibilities even though other ecosystems call them encodings.

Codec may provide primitive encodings used inside those formats, but should not grow an alternative parser/serializer family. No custom parser or private format DSL is introduced.

## 5. API character

Codec operations are explicit transforms with precise input and output types. Names should identify both operation and representation where ambiguity exists.

Strict decoding is the default. Lenient or whitespace-tolerant modes must be explicit and documented.

Errors report position and reason where practical. Decoders should bound allocation based on input size.

## 6. Character encodings

UTF conversion needs explicit unit semantics:

- UTF-8 bytes;
- UTF-16 code units with stated endianness;
- UTF-32 code units with stated endianness;
- Unicode scalar/rune sequences where boru has such a carrier;
- invalid-sequence policy: reject, replace or preserve, selected explicitly.

Do not use the term "character" without identifying whether it means byte, code unit, scalar value or grapheme.

## 7. Type checking and bytecode

Codec is largely pure. Precise signatures should distinguish String, Bytes and typed representation values.

Literal pure transforms may be constant-folded when doing so is bounded and target-independent. Folding must enforce the same validation and errors as runtime execution.

Host endianness must not influence explicit codec operations. The requested encoding's byte order is part of the operation, not inferred from the machine.

## 8. Testing

Use published vectors and round-trip properties. Include malformed, truncated, non-canonical and resource-boundary inputs.

Every export receives language-level spec coverage, with Go tests for implementation details and fuzzing for decoders.

## 9. Open questions

1. Exact inventory of existing BinUtil functionality.
2. Ownership of URL percent encoding.
3. Whether ASN.1 belongs in initial scope.
4. Carrier for Unicode scalar sequences.
5. Canonical versus permissive decoding policy per encoding.
