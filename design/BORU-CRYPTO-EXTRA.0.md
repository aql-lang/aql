# `boru:crypto` Platform Module

**Status:** Discovery draft  
**Module:** `boru:crypto`  
**Purpose:** Cryptographic primitives, safe recipes and interoperable security formats

## 1. Positioning

BORU is batteries-included. The crypto module should not merely reproduce Go's package structure or Node's low-level object APIs. It should expose a coherent BORU façade over host cryptography, combining precise primitives with common modern recipes.

The module must retain legacy algorithms such as MD5 because interoperability and data identification sometimes require them. Legacy algorithms are available but never selected by safe defaults and are marked clearly in metadata and documentation.

Crypto explains **how** cryptographic operations are performed. Vault explains **where** secrets and keys are stored, rotated and authorized. A future Trust module will integrate OS keychains, secure hardware, passkeys and user-verification capabilities.

## 2. Layering

### 2.1 Primitive layer

- cryptographically secure random bytes;
- hash functions;
- message authentication codes;
- key-derivation functions;
- authenticated encryption;
- public-key encryption where justified;
- signing and verification;
- key agreement;
- constant-time primitives.

### 2.2 Recipe layer

- password hashing and verification;
- secure token generation;
- TOTP/HOTP;
- JWT signing, verification and inspection;
- shared-secret generation and splitting;
- key fingerprints;
- structured tokenization/redaction helpers, provisionally retained here.

### 2.3 Integration layer

- key import/export;
- PEM, DER and JWK/JWKS integration through codec facilities;
- opaque vault/HSM/host key handles;
- algorithm and key metadata.

## 3. Algorithm registry

Use one coherent algorithm registry rather than a module per algorithm.

Each algorithm has metadata such as:

- canonical name and aliases;
- category;
- output/key/nonce constraints;
- current status: recommended, acceptable, legacy or prohibited-by-policy;
- streaming support;
- host availability;
- applicable key types.

Policy may disable algorithms without pretending they do not exist.

## 4. Hashing and MAC

Provide one-shot and streaming hashing. Digests should carry algorithm identity rather than being indistinguishable raw bytes.

HMAC generation and verification are first-class. Verification uses constant-time comparison internally.

Legacy hashes such as MD5 and SHA-1 are allowed for checksums, fingerprints of existing systems and protocol compatibility. Documentation and metadata must state that they are not suitable for password storage, signatures or collision-resistant security decisions.

## 5. Passwords and KDFs

Provide safe recipe APIs for password hashing and verification. Defaults are versioned profiles so parameters can evolve.

Candidate KDF support includes HKDF, PBKDF2, scrypt and Argon2 where host implementation and policy permit.

Password verification returns structured information that can indicate both match and whether rehashing with current parameters is recommended.

Passwords and secret material should use secret-aware carrier types or handles where feasible to reduce accidental logging and rendering.

## 6. Authenticated encryption

AEAD is the default symmetric encryption surface. APIs manage nonce requirements explicitly and should offer a safe generated-nonce recipe.

The result should be a structured sealed value containing required metadata rather than an ambiguous byte string. Raw primitives remain available for interoperability.

Unauthenticated encryption modes are legacy/advanced and require explicit algorithm choice and policy permission.

## 7. Keys

Key values are strongly distinguished by purpose and representation:

- symmetric secret,
- public key,
- private key,
- signing key,
- verification key,
- agreement key,
- opaque managed-key handle.

A vault-managed or hardware-managed key implements the same operation-facing protocol as an in-memory key. Call sites can therefore invoke signing or decryption without knowing where the private key lives.

Private key export is never implied by the ability to use a key.

## 8. Signatures and key agreement

Signing and verification pin an algorithm or accepted algorithm set. Verification never trusts algorithm declarations from untrusted input without policy validation.

Key agreement returns derived secret material through an explicit KDF step or a recipe that includes one. Raw shared points/secrets should not become application keys accidentally.

## 9. Secure tokens

Token generation is first-class.

A token profile defines:

- semantic prefix,
- version,
- random body entropy,
- alphabet/encoding,
- optional checksum for transcription safety,
- rendering and redaction rules.

The prefix conveys type, not security. The random body supplies unpredictability. Verification helpers compare stored token hashes rather than requiring plaintext token storage.

Profiles may produce API keys, session tokens, reset tokens and other opaque identifiers. BORU ID may consume crypto randomness but semantic secret tokens remain crypto recipes.

## 10. OTP

Provide HOTP and TOTP generation and verification with explicit parameters and interoperable defaults.

Support:

- secret generation;
- provisioning URI construction/parsing;
- configurable digit count and period;
- bounded clock-skew/window verification;
- replay-prevention guidance.

OTP APIs must distinguish generating a display code from verifying one and must not silently accept unlimited windows.

## 11. JWT

JWT utilities belong in the recipe layer.

Provide:

- sign;
- verify;
- inspect/decode without trust;
- claim-validation helpers;
- key-set resolution behind explicit network/policy boundaries.

Security requirements:

- reject `none`;
- pin allowed algorithms;
- reject algorithm/key confusion;
- validate expiration, not-before, issuer and audience when configured;
- define clock skew explicitly;
- distinguish decoded claims from verified claims;
- bound remote JWKS fetching and caching;
- begin with signed JWT/JWS; defer JWE unless a concrete use case justifies it.

## 12. Shared secrets

Provide utilities for:

- secure secret generation;
- derivation and domain separation;
- wrapping/unwrapping;
- secret comparison;
- threshold splitting and reconstruction;
- key agreement recipes;
- secret lifecycle metadata.

Threshold secret sharing must state scheme, threshold and share identity in each share. Reconstruction rejects duplicates and incompatible share sets.

## 13. Structured tokenization and data protection

For now, structured tokenization remains in `boru:crypto`, marked for reconsideration.

Potential operations traverse maps/lists and apply a declared policy:

- redact;
- mask;
- deterministic HMAC pseudonymization;
- randomized token replacement;
- encrypt selected fields;
- restore vault-backed tokens.

This area is broader than cryptography and may later become `boru:privacy` or another module. It must not be confused with execution context or identifier generation.

## 14. Constant-time behaviour

Do not offer a callback wrapper that claims to make arbitrary BORU code constant-time. The runtime, compiler, garbage collector, data-dependent allocations and called code make such a guarantee unsound.

Instead:

- guarantee constant-time behaviour for narrowly defined native primitives;
- provide constant-time equality for equal-length secret byte values;
- provide constant-time select/copy/xor primitives only when justified;
- use those primitives internally in higher-level verification APIs;
- document length leakage where unavoidable.

A general "run this callback in constant time" API is rejected.

## 15. Micron usage

Microns are suitable for small semantically rich crypto values where they improve safety and readability, such as algorithm identities, compact digest/signature descriptors, key identifiers, nonce descriptors or token metadata.

Large ciphertexts, arbitrary binary payloads and substantial key material remain binary or opaque handle types. Microns should communicate semantics, not become a container for all bytes.

## 16. Vault relationship

The target architecture is:

- `boru:crypto` supplies all algorithms and recipes;
- Vault is primarily BORU business logic using `boru:crypto`;
- host-specific Go crypto remains hidden behind the module;
- Vault adds storage, policy, rotation, audit and provider integration;
- managed key references can be passed directly to crypto operations.

Anything generically required by Vault should be considered for the crypto surface unless it is truly storage/provider-specific.

## 17. Trust relationship

OS keychains, secure enclaves, biometric/user-presence prompts, passkeys and hardware authentication do not belong directly in OS or crypto.

A future `boru:trust` module should expose semantic operations such as user presence, user verification, credential creation and opaque hardware-bound key handles. Those handles flow into crypto signing/decryption APIs.

## 18. Type checking

Use distinct carrier types for digest, signature, nonce, ciphertext, secret, token, key and managed-key handle. Algorithm compatibility should be statically checked where types are concrete.

Do not accept generic bytes for every parameter merely because the host library does. Dynamic algorithm selection remains possible but returns validated typed values or structured errors.

Sensitive values should have safe default rendering and should not interpolate into logs as plaintext.

## 19. Bytecode

Crypto operations execute at runtime. Do not constant-fold secret generation, signatures, encryption, password hashing or random tokens.

Pure public transforms such as hex encoding may be folded in codec, but crypto results should generally remain runtime operations to preserve randomness, policy and provider selection.

Opaque key handles and secret values require VM-safe lifetime and cleanup semantics. Native errors become BORU errors, never panics.

## 20. Testing

Every export needs formal module spec coverage. Deterministic test vectors cover algorithms and formats. Randomized recipes use structural/property assertions and injectable deterministic test sources only in test-only privileged paths.

Negative tests are essential: wrong key, modified ciphertext, invalid nonce, disallowed algorithm, expired JWT, wrong audience, malformed share sets and policy denial.

## 21. Open questions

1. Final names and profile syntax.
2. Exact type representation for secret values.
3. Policy location and configuration.
4. Whether structured tokenization remains in crypto.
5. Initial algorithm baseline.
6. Provider interface for Vault, HSM and host trust handles.
7. Whether post-quantum KEM/signature support is included initially or registry-ready only.
