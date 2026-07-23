# `aql:id` Identifier Module

**Status:** Discovery draft  
**Module:** `aql:id`  
**Purpose:** Generate, parse, validate, inspect and normalize identifiers

## 1. Positioning

`aql:id` is a utility module for identifiers, not secret bearer tokens. It uses `aql:crypto` when unpredictability is required, but its primary concerns are uniqueness, ordering, readability, interoperability and naming.

The preferred API is profile-oriented. Developers ask for a semantic profile such as sortable, random, compact or human-safe rather than configuring a large collection of algorithm knobs for routine cases.

## 2. Identifier families

The module should support standard and widely used families where licenses and specifications permit, including:

- UUID/GUID versions with explicit version choice;
- time-ordered UUID forms;
- ULID-like sortable identifiers;
- compact alphabet-based identifiers such as NanoID-style generation;
- deterministic namespace identifiers;
- monotonic/local sequence profiles where appropriate;
- application identifiers with semantic prefixes;
- short-lived or throwaway non-secret identifiers.

The exact initial list requires a standards survey. Each family has explicit parsing, validation and canonical rendering rules.

## 3. Profiles

Candidate profiles include:

- **standard:** interoperable standard identifier;
- **random:** cryptographically unpredictable identifier;
- **sortable:** lexically time-sortable identifier;
- **compact:** configurable length/alphabet with calculated collision properties;
- **deterministic:** namespace plus input produces stable output;
- **human-safe:** avoids ambiguous characters and culturally unsafe outputs;
- **name:** readable adjective-noun or similar generated names;
- **test:** deterministic seeded generation, clearly marked non-production.

Profiles select safe defaults while advanced callers may select a concrete algorithm.

## 4. Prefixes and domains

Identifiers may include a semantic prefix indicating entity type, for example a request, account or order. Prefixes improve debugging but are not secret and do not contribute meaningful entropy.

The module should define canonical separators, prefix validation and total-length calculation. Secret API tokens with prefixes remain in `aql:crypto`; ordinary entity IDs belong here.

## 5. Inspection and validation

Provide operations to:

- identify a known format;
- parse to a typed identifier value;
- validate canonical and non-canonical forms;
- normalize where a lossless canonical representation exists;
- extract metadata explicitly encoded by the format;
- compare or order identifiers according to format semantics;
- report estimated entropy/collision properties for configured compact profiles.

Inspection must not claim metadata that the format does not encode. In particular, a random identifier has no trustworthy creation time unless its standard includes one.

## 6. Human-safe identifiers

Human-facing identifiers require more than removing a short profanity list.

A profile may control:

- allowed alphabet;
- ambiguous-character removal;
- accidental word detection across boundaries;
- prohibited terms and protected-group slurs;
- common obfuscations and misspellings;
- locale;
- maximum regeneration attempts;
- policy version.

Filtering is best-effort, not a guarantee that no person will ever find an output objectionable. Documentation must say so.

For compact random strings, filters must account for the entropy loss caused by rejection. The generator retries with a bounded strategy and fails explicitly if policy is impossible.

## 7. Throwaway readable names

Provide generated names suitable for branch names, temporary environments, repositories, examples and local tasks, such as adjective-noun combinations.

The generator should support:

- separator choice;
- optional numeric suffix;
- target length;
- deterministic seeded mode for tests;
- locale/profile selection;
- pluralization or grammatical pattern only if word metadata supports it;
- collision suffixing;
- safety filtering.

Word lists should be curated for clarity, broad cultural safety and low ambiguity. "Culturally safe" remains a policy profile, not an absolute claim.

## 8. Embedded lexicon representation

The built-in lexicon must be small and should not appear as casually greppable plain text in source or binary form.

Possible compiled representations include:

- front-coded string tables;
- indexed byte pools;
- minimal perfect hash tables;
- tries or finite-state automata;
- compressed blocks generated at build time.

The representation is an implementation detail, not a security boundary. If the runtime can generate a word, a determined analyst can recover it.

Source word lists may be stored outside ordinary source files and compiled by a generator, subject to repository policy. Generated data should be reproducible and reviewable through tooling even if the shipped representation is opaque.

## 9. Extensible lexicons and safety policy

Applications can register additional lexicons or policies without mutating the immutable built-in set.

Extensions should be scoped to an engine/application context and carry metadata:

- locale;
- part of speech;
- safety class;
- source/version;
- weighting;
- allowed usage profiles.

Policy composition needs deterministic precedence. Removing a built-in blocked term should require an explicit privileged override, while adding blocked terms is ordinary application configuration.

Untrusted packages must not silently weaken a host's safety policy.

## 10. Fuzzy unsafe-term detection

Detection may include normalized forms, separator removal, repeated-character folding, common substitutions and limited edit-distance matching.

The design must control false positives and denial-of-service risk. A compiled automaton or indexed normalized blocklist is preferable to running many regular expressions per candidate.

Generation should filter both individual words and their concatenated rendered form, because unsafe terms can appear across word/separator boundaries.

## 11. Entropy and collision guidance

For configurable IDs, the module should calculate or expose approximate entropy and birthday-bound collision guidance.

Profiles must set minimums for intended use. A "throwaway name" profile may tolerate collisions and provide suffixing; a distributed entity ID profile must not.

Do not market readable names as cryptographic secrets.

## 12. Relationship to crypto

`aql:id` consumes cryptographically secure randomness from `aql:crypto` for random profiles. It does not reimplement random number generation.

Deterministic cryptographic namespace IDs may use approved hashes through crypto. Secret token hashing/verification remains in crypto.

## 13. Types and Microns

Identifiers should use typed values carrying family and metadata rather than plain strings wherever practical.

Microns may represent compact identifier descriptors, versions, prefixes or small IDs when their size and semantics fit. Large or variable identifiers may use a dedicated typed string/bytes carrier.

Rendering to String is explicit and canonical.

## 14. Bytecode

Random and time-based generation occurs at runtime and is not folded. Deterministic generation from literal inputs may be folded only if the algorithm/profile version is pinned and folding cannot embed host-specific state.

Lexicon profiles and policy versions used by compiled artifacts must be stable or resolved at runtime deliberately.

## 15. Testing

- Published standard vectors.
- Round-trip parse/render tests.
- Monotonicity and ordering tests.
- Collision/property tests at bounded scale.
- Deterministic seeded name tests.
- Safety-filter test corpus including obfuscations and cross-boundary terms.
- Extension and policy-precedence tests.
- Fuzzing for parsers and normalization.

Every public export requires language-level spec coverage.

## 16. Open questions

1. Initial standard algorithm set.
2. Canonical typed identifier representation.
3. Exact module API names.
4. Lexicon source governance and update process.
5. Locale support in the first release.
6. Policy override permissions.
7. Whether collision/entropy estimates are values, diagnostics or documentation only.
8. Whether branch/repository naming deserves a dedicated named profile.
