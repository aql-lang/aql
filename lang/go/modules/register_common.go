package modules

import (
	"fmt"

	"github.com/aql-lang/aql/lang/go/native"
)

// Shared register/idempotency machinery for the DSL "register" words —
// ParseLang.register, MiniLang.register, EmitLang.register. All three install
// an AQL fn as a kind (`parse_<name>` / `lang_<name>` / `emit_<name>`) and must
// treat the COMPILED path's double execution as ONE registration while still
// rejecting a genuine double-register. Keeping the logic here guarantees the
// three registrars behave identically.

// registerIdent tracks a dynamically-registered kind's install identity.
//
// The compiled path installs a kind twice — once in the check-mode ReturnsFn
// (so `<verb> <name>` resolves statically during the check/compile pass) and
// once when the VM re-runs the same `register` CALL — and those two must be one
// logical registration. A source-Pos identity alone can't tell that apart from
// a genuine repeat, because a LOOP body re-executing the same `register` line
// also carries the same Pos. So we additionally record the PHASE: the install
// done in check mode is idempotent for exactly ONE subsequent runtime re-run
// (the VM CALL), after which `fromCheck` flips and any further runtime
// re-execution — a loop's next iteration — collides like any other
// double-register.
type registerIdent struct {
	pos       string // "row:col:src" of the registering fn value; "::" == no Pos
	fromCheck bool   // installed during the check pass; consumed by the one VM re-run
}

// registerIdentity is the source-call identity of a register: the Pos of the
// registering fn value. Two calls over the same source token share it; "::"
// (no Pos) is the zero identity and is never treated as idempotent.
func registerIdentity(fn native.Value) string {
	p := fn.Pos
	return fmt.Sprintf("%d:%d:%s", p.Row, p.Col, p.Src)
}

// registerCollisionInstall is the shared install + idempotency body. `key` is
// the validated export key, `fn` the registering fn value, `checkMode` is
// r.Check.IsActive() at the call site, and `existsErr` builds the
// module-specific `<kind>_kind_exists` error. It returns nil on a fresh install
// or a legitimate idempotent re-run, and the kind_exists error on a genuine
// double-register (a second source call, or a loop re-executing the same line
// at runtime). A kind present in `exports` but absent from `idents` is a
// built-in / host registration with no tracked identity, so re-registering it
// always collides.
func registerCollisionInstall(exports *native.OrderedMap, idents map[string]registerIdent, key string, fn native.Value, checkMode bool, existsErr func() error) error {
	id := registerIdentity(fn)
	if _, exists := exports.Get(key); exists {
		existing, tracked := idents[key]
		if tracked && id != "::" && existing.pos == id {
			if checkMode {
				return nil // re-analysis of the same register call during check — no-op
			}
			if existing.fromCheck {
				existing.fromCheck = false
				idents[key] = existing // the ONE VM re-run of the check-install — idempotent, now consumed
				return nil
			}
			// Runtime re-execution of the same source call that is NOT the
			// check-install's VM re-run — a loop body's later iteration — is a
			// genuine double-register and falls through to the error.
		}
		return existsErr()
	}
	exports.Set(key, fn)
	idents[key] = registerIdent{pos: id, fromCheck: checkMode}
	return nil
}

// surfaceRegisterCheckError reports a register install failure that a
// check-mode ReturnsFn would otherwise discard — a bad name / signature, or a
// genuine double-register the checker can see statically — as an error-severity
// check diagnostic, so `aql check` flags it instead of silently keeping the
// first install and passing. `at` supplies the source position (the kind-name
// token). No-op when there is nothing to report.
func surfaceRegisterCheckError(r *native.Registry, err error, at native.Value) {
	if r == nil || err == nil {
		return
	}
	code, detail := "register_error", err.Error()
	if ae, ok := err.(*native.AqlError); ok {
		code, detail = ae.Code, ae.Detail
	}
	r.Check.AddDiagnostic(native.CheckDiagnostic{
		Code:     code,
		Detail:   detail,
		Word:     "register",
		Row:      at.Pos.Row,
		Col:      at.Pos.Col,
		Severity: native.SeverityError,
	})
}
