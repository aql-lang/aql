# Dollar-Prefixed System Names

**Status:** Speculative; additional design work required  
**Scope:** Cross-language naming convention

## 1. Motivation

BORU would benefit from a language-wide visual convention that tells a developer, immediately and consistently, that a name belongs to the language or runtime rather than to ordinary application code. Dollar-prefixed names are a plausible convention.

Potential uses include protected execution-context slots, compiler metadata, runtime tags and future system annotations. A single convention reduces surprise: wherever a developer sees a bare `$name`, it denotes a system concern.

## 2. The conflict

External data is allowed to contain keys beginning with `$`. JSON documents, database records and third-party protocols commonly use such names. BORU must preserve those keys exactly and permit ergonomic access to them.

A global prohibition on every string beginning with `$` would break lossless data interchange. A rule that applies only in selected containers, however, risks becoming a visible non-uniformity.

## 3. Candidate rule

The leading candidate is a distinction between **language names** and **data keys**:

- A bare name beginning with `$` is reserved for the system.
- A quoted string such as `"$foo"` remains ordinary data.
- A data container has one canonical key `"$foo"`; the syntax must not create a second parallel key merely because one spelling is bare and one is quoted.
- Ordinary user code cannot define, bind or mutate a protected bare `$name`.
- Parsers and serializers preserve external dollar-prefixed string keys losslessly.

This preserves the general visual rule while allowing external data.

## 4. Ergonomic requirements

The design is acceptable only if ordinary work with external `$` keys remains comfortable.

The language and formatter should:

- accept quoted-key access consistently in maps and flex values;
- preserve the quoted spelling when required;
- produce a clear diagnostic when a bare reserved spelling is attempted;
- avoid silently redirecting a bare system name to a string key;
- provide ordinary reflection/enumeration over the actual string keys;
- round-trip JSON and similar formats without rewriting keys.

## 5. Alternatives

### Reserve by container

Reserve `$` only in execution-context or metadata stores. This is mechanically simple but weakens the global convention and may create a NUR-worthy container exception.

### Reserve by internal key type

Represent system slots using an internal key type that merely renders with `$`. This strongly separates system state from user strings, but the surface syntax and introspection rules need care.

### Do not reserve `$`

Keep `$` as convention only. This provides no protection against accidental or malicious mutation and therefore does not meet the execution-context security requirement.

## 6. Current disposition

No final rule is selected.

The inherited execution-context design must not depend on user-visible `$` semantics until this note is resolved. It may use protected typed slots internally.

Before implementation, inspect current parser, binder, flex/map access and formatter behaviour for bare and quoted dollar-prefixed names. If the selected rule introduces a genuine exception to an otherwise uniform naming or key-access rule, record it through the NUR process.

## 7. Open questions

1. Are dollar-prefixed bare atoms currently legal in every binding position?
2. Does `/q` or equivalent quoting produce a string key, atom key or another canonical form?
3. How should dot access render or access a dollar-prefixed data key?
4. Can formatter output always preserve the distinction?
5. Which system entities may introduce protected dollar names?
6. Is a protected name readable but not writable, or inaccessible except through dedicated words?
