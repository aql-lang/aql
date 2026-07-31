# boru Vault — MFA / TOTP Codes (Design)

**Status: design only — not implemented.** This note captures the
shape of the feature, the security tradeoff that gates it, and a
recommended implementation path, so the decision can be made
deliberately later.

## 1. Problem

Some package publishers require a one-time MFA code *in addition to* the
API token when you push a release. The motivating case is RubyGems:

```
$ gem push pkg.gem
You have enabled multifactor authentication. Please enter OTP code.
Code:
```

The vault already injects the long-lived token (`boru vault exec
--for=gem=gem_token -- gem push …`), but the interactive OTP prompt
breaks an otherwise unattended flow. The question: can the vault also
produce the OTP, given the user already has MFA configured on a phone?

## 2. Background — what the OTP is

The publishing OTP is almost always **TOTP** (RFC 6238): a 6-digit code
derived from a shared **seed** (base32, provisioned as an
`otpauth://totp/…?secret=…` URI / QR at enrollment) plus the current
time, in 30-second windows. It is `HOTP(seed, floor(unixtime/period))`
truncated to N digits (HOTP = RFC 4226, HMAC-SHA1 by default).

Two consequences matter for this design:

- **The code is derived, not stored.** Anyone with the seed can compute
  every future code. The seed is the secret; the 6 digits are ephemeral
  (valid ~30 s) and are *not* themselves sensitive to store or log.
- **The seed lives wherever you enrolled it.** If MFA was set up only on
  a phone, the vault does not have the seed and cannot generate codes.
  Password managers (1Password, Bitwarden) that "do TOTP" work precisely
  by holding the same seed the authenticator app holds.

## 3. The security tradeoff (the real decision)

MFA's value is that the second factor lives on a **separate possession**
(the phone). If the TOTP seed is stored in the *same vault* as the API
token, then unlocking that vault yields **both factors at once** — 2FA
collapses back to 1FA, with the vault passphrase as the single thing
protecting the account. A compromised dev machine that can unlock the
vault then holds everything.

This is not automatically wrong — it is the same bargain every
password-manager-with-TOTP makes, and it is still far better than a
plaintext token in `~/.gem/credentials`. But for a *publishing*
credential it specifically weakens the "stolen / compromised laptop"
story, and that should be stated plainly to anyone who turns it on:
storing the seed beside the token raises the bar versus plaintext, but
it is **not** equivalent to keeping the second factor on a phone.

Middle grounds, if some separation is wanted back:

- Keep the seed in a **different vault** (separate passphrase / unlock
  policy / backend) from the token.
- Require an **interactive confirmation** (or a touch / re-auth) before
  emitting a code, so a silent background process can't harvest codes.
- **Don't store the seed at all** — let the vault only *relay* a code
  the user types from the phone. Then the vault isn't "handling" MFA,
  just plumbing it, and provides little value.

The feature should make the collapse explicit (docs + a one-time warning
on enrollment), not hide it.

## 4. Seed acquisition (the practical wrinkle)

"MFA is already on my phone" usually means the seed exists *only* on the
phone; authenticator apps generally don't export it. To let the vault
generate codes the user must obtain the same seed:

- **Re-enroll** MFA (disable + re-add) and, at the QR step, capture the
  `otpauth://…?secret=BASE32…` value (most sites show a manual-entry key
  beside the QR), or
- **Multi-enroll** at setup time: the same secret can be added to
  several authenticators — scan into the phone *and* store the secret in
  the vault in the same sitting.

There is no way around this: the vault must hold the same seed the phone
holds. The design cannot extract it from an existing phone enrollment.

## 5. Integration shapes

The OTP integrates differently from the publish-token recipes:

- The **API token** is consumed via **environment variables** — exactly
  what `exec --for=<recipe>` already does (`recipes.go`).
- The **OTP** is consumed as a **command-line flag or an interactive
  prompt**, not an env var:

| Tool      | OTP delivery                         | Notes |
|-----------|--------------------------------------|-------|
| RubyGems  | `gem push --otp CODE` (else prompts) | the motivating case |
| npm       | `npm publish --otp=CODE`             | when 2FA-for-writes is on; automation tokens skip it |
| PyPI / `twine` | — none —                        | pure token auth; 2FA only guards the web UI |
| cargo     | — none —                             | token auth, no OTP on publish |

So this is really a **RubyGems / npm** concern, and the natural delivery
is a *flag value*, not an injected env var. That points away from the
recipe/env model and toward a small generator primitive.

## 6. Proposed design

### 6.1 Primary: a `boru vault otp <alias>` subcommand

A generator that prints the current code, composed into the publish
command via shell substitution:

```bash
# store the seed once (paste the otpauth:// URI or the base32 secret)
boru vault add --from-stdin gem_mfa

# generate a live code on demand:
gem push --otp "$(boru vault otp gem_mfa)" pkg.gem
npm publish --otp="$(boru vault otp npm_mfa)"
```

Behaviour: unlock (reuse the standard auth path) → read the seed for
`<alias>` → compute `TOTP(seed, now)` → print the digits to stdout.

Why a subcommand rather than a recipe:

- **Universal.** Works with any tool that takes an `--otp`-style flag,
  with no per-tool plumbing.
- **Right delivery.** The code reaches the tool as a flag, the form the
  tools actually want.
- **Nothing leaks to a long-lived env var.** The code is on the argv of
  one short-lived process and expires in ~30 s.
- **Composes with the token path.** Token via
  `exec --for=gem=gem_token -- gem push --otp "$(boru vault otp gem_mfa)"
  …`; the repeatable `--for` (already shipped) carries the token while
  the substitution supplies the OTP.

The generated code is *derived and ephemeral*, so unlike secret values
it can be printed to stdout. The seed, of course, is never printed.

### 6.2 Optional: an OTP recipe for env-reading tools

For any tool that *does* read an OTP from an env var, the same generator
can back an `--for`-style recipe whose `env()` computes the code at read
time (instead of returning a stored secret). This reuses the existing
recipe machinery and the repeatable `--for`:

```bash
boru vault exec --for=gem=gem_token --for=gem-otp=gem_mfa -- gem push …
```

This is a thin add-on over §6.1 and should follow it, not lead — the
flag form covers the known cases (gem, npm).

### 6.3 Storage & metadata

- The seed is stored as an ordinary vault secret (its value is the
  base32 seed, or the full `otpauth://` URI which the generator parses
  for non-default digits/period/algorithm).
- Optionally add a lightweight **`--kind=totp`** marker on the `Alias`
  (`store.go` `Alias` currently has `Provider`, `Source`, `ExpiresAt`,
  …). A kind lets `vault list` and the TUI detail page show "OTP seed"
  and offer a "current code" action, and lets `otp` refuse a non-TOTP
  alias with a clear error. Recommendation: ship §6.1 treating the value
  as a seed (no schema change); add the `kind` tag in a follow-up.

## 7. Implementation sketch (for when this is built)

Small and std-lib only:

- **TOTP core** (~40 lines): `crypto/hmac` + `crypto/sha1`,
  `encoding/base32`, big-endian time counter, dynamic truncation to N
  digits (RFC 4226 §5.3 / RFC 6238).
- **`otpauth://` parsing**: read `secret`, and optional `digits`,
  `period`, `algorithm` (support SHA1 default; SHA256/512 trivially).
- **Subcommand wiring**: a `runOTP(args, homeDir, …)` in
  `cmd/go/internal/vault/`, dispatched from `vault.go`'s `Run()` switch,
  reusing `requireStore` + `authenticate` + `sess.getValue`.
- **Audit**: append a `vault.otp` event (alias + outcome) — never the
  code, never the seed.
- **Clock**: TOTP needs a roughly accurate clock (±1 step is usually
  tolerated server-side); note this in help. No special handling needed.
- **Tests**: RFC 6238 published test vectors (the appendix seed and
  timestamps) give deterministic unit tests for the core; a CLI test for
  add-seed → `otp` → 6-digit output and a non-TOTP-alias error.

Out of scope for v1: HOTP (counter-based), Steam/other custom alphabets,
push/U2F/WebAuthn (not derivable from a stored seed).

## 8. Alternatives & non-goals

For **unattended / CI** publishing the better answer is usually to avoid
interactive 2FA entirely, and the ecosystem is moving that way:

- **Trusted Publishing / OIDC.** PyPI, RubyGems, and npm (provenance)
  can mint a short-lived publish credential from a CI job's OIDC
  identity — no long-lived token *and* no OTP. This is the preferred
  path for automation and is out of scope for this feature.
- **Automation / granular tokens.** npm "automation" tokens and
  RubyGems API keys can be scoped to skip MFA-for-writes.

The `boru vault otp` subcommand targets the *interactive local publish*
case — where you'd otherwise reach for your phone every push — not CI.
PyPI/`twine` and cargo are non-goals (no OTP on upload).

## 9. Open questions

- Do we ship the `kind=totp` schema tag in v1 or defer (recommendation:
  defer)?
- Should `otp` require a re-confirmation / fresh auth each call to blunt
  the silent-harvest risk (§3), or rely on the session like other ops?
- Worth a TUI "current code" action on the detail page (with a 30 s
  countdown), or keep this CLI-only initially?
