package vault

import "fmt"

// Scoped-password authorization model.
//
// A vault password slot carries a Scope (one tier) and a namespace
// allow-list. Namespace isolation is cryptographic — a session only
// holds the data keys (NDKs) for namespaces sealed to its slot, so it
// physically cannot decrypt an unheld namespace. The read/write/move
// tier within a held namespace is enforced here by policy + audit
// (symmetric key possession can't separate read from write); to stop a
// non-admin holder editing the plaintext Scope/Namespaces in
// vault.jsonic to self-escalate, those fields are cryptographically
// bound into the slot's Verifier and EncPrivKey AAD (see keyslot.go).

// Scope tiers. admin ⊇ everything; write and move each imply read; read
// is the floor.
const (
	ScopeAdmin = "admin"
	ScopeWrite = "write"
	ScopeMove  = "move"
	ScopeRead  = "read"
)

// Op is the minimum authorization an operation requires.
type Op int

const (
	OpRead Op = iota
	OpWrite
	OpMove
	OpAdmin
)

// String renders an Op for error messages.
func (o Op) String() string {
	switch o {
	case OpRead:
		return "read"
	case OpWrite:
		return "write"
	case OpMove:
		return "move"
	case OpAdmin:
		return "admin"
	default:
		return "unknown"
	}
}

// validScope reports whether s is one of the four scope tiers.
func validScope(s string) bool {
	switch s {
	case ScopeAdmin, ScopeWrite, ScopeMove, ScopeRead:
		return true
	}
	return false
}

// scopeAllows reports whether a slot with the given scope may perform
// op. admin allows everything; write allows read+write; move allows
// read+move; read allows only read. write and move are siblings — a
// write slot may not move and a move slot may not write.
func scopeAllows(scope string, op Op) bool {
	switch scope {
	case ScopeAdmin:
		return true
	case ScopeWrite:
		return op == OpRead || op == OpWrite
	case ScopeMove:
		return op == OpRead || op == OpMove
	case ScopeRead:
		return op == OpRead
	}
	return false
}

// slotCoversNamespace reports whether slot is authorized for namespace
// ns (stored form; "" is root). A nil slot is the legacy implicit-admin
// session, which covers everything; an empty or ["*"] allow-list covers
// every namespace. This is the policy-level check; the cryptographic
// check is whether the session actually holds ns's NDK.
func slotCoversNamespace(slot *PasswordSlot, ns string) bool {
	if slot == nil {
		return true
	}
	if len(slot.Namespaces) == 0 {
		return true
	}
	for _, n := range slot.Namespaces {
		if n == "*" || n == ns {
			return true
		}
	}
	return false
}

// requireScope enforces the operation tier against the session's scope.
// A legacy session (no slot) is implicit admin and always passes.
func requireScope(sess *Session, op Op) error {
	if sess.legacy() {
		return nil
	}
	if !scopeAllows(sess.Scope, op) {
		return fmt.Errorf("permission denied: %s requires a higher scope (this password has %q scope)", op, sess.Scope)
	}
	return nil
}

// requireScopeNS enforces the operation tier and that the session covers
// (and cryptographically holds a key for) namespace ns.
func requireScopeNS(sess *Session, op Op, ns string) error {
	if err := requireScope(sess, op); err != nil {
		return err
	}
	if sess.legacy() {
		return nil
	}
	if !slotCoversNamespace(sess.Slot, ns) {
		return fmt.Errorf("permission denied: this password does not cover namespace %q", nsLabel(ns))
	}
	return nil
}
