package vault

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func mustRandom(t *testing.T, n int) []byte {
	t.Helper()
	b, err := newRandom(n)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestValueSealRoundTrip seals a value under an NDK and opens it back,
// and confirms the wrong NDK / relabeled namespace / relabeled alias all
// fail (AAD binding).
func TestValueSealRoundTrip(t *testing.T) {
	ndk := mustRandom(t, ndkLen)
	id, _ := newNDKID()
	sealed, err := sealValue(ndk, id, "proj", "proj:apikey", "sk-secret")
	if err != nil {
		t.Fatal(err)
	}

	resolve := func(want []byte) func([]byte) ([]byte, bool) {
		return func(got []byte) ([]byte, bool) {
			if !bytes.Equal(got, id) {
				return nil, false
			}
			return want, true
		}
	}

	got, err := openValue(sealed, "proj", "proj:apikey", resolve(ndk))
	if err != nil {
		t.Fatalf("openValue: %v", err)
	}
	if got != "sk-secret" {
		t.Fatalf("roundtrip = %q, want sk-secret", got)
	}

	// Wrong NDK bytes -> decryption fails.
	if _, err := openValue(sealed, "proj", "proj:apikey", resolve(mustRandom(t, ndkLen))); err == nil {
		t.Error("openValue with wrong NDK should fail")
	}
	// Relabeled namespace -> AAD mismatch.
	if _, err := openValue(sealed, "other", "proj:apikey", resolve(ndk)); err == nil {
		t.Error("openValue with relabeled namespace should fail")
	}
	// Relabeled alias -> AAD mismatch.
	if _, err := openValue(sealed, "proj", "other:apikey", resolve(ndk)); err == nil {
		t.Error("openValue with relabeled alias should fail")
	}
	// No key for the id -> ErrNoKeyForValue (distinct from corruption).
	if _, err := openValue(sealed, "proj", "proj:apikey", func([]byte) ([]byte, bool) { return nil, false }); err != ErrNoKeyForValue {
		t.Errorf("missing key error = %v, want ErrNoKeyForValue", err)
	}
}

// TestNDKBox seals an NDK to a slot public key and opens it with the
// keypair; a different keypair must fail.
func TestNDKBox(t *testing.T) {
	pub, priv, err := newSlotKeypair()
	if err != nil {
		t.Fatal(err)
	}
	ndk := mustRandom(t, ndkLen)
	sealed, err := sealNDK(ndk, pub)
	if err != nil {
		t.Fatal(err)
	}
	got, err := openNDK(sealed, pub, priv)
	if err != nil {
		t.Fatalf("openNDK: %v", err)
	}
	if !bytes.Equal(got, ndk) {
		t.Fatal("NDK roundtrip mismatch")
	}
	otherPub, otherPriv, _ := newSlotKeypair()
	if _, err := openNDK(sealed, otherPub, otherPriv); err == nil {
		t.Error("openNDK with the wrong keypair should fail")
	}
}

// TestPrivKeyBindsScope is the crypto-layer proof of H2: the private key
// opens only under the exact Scope it was sealed with, so flipping the
// plaintext Scope breaks authentication. Namespaces are deliberately NOT
// bound (they are gated by NDK possession), so changing the namespace
// list does not affect the private-key AAD.
func TestPrivKeyBindsScope(t *testing.T) {
	kek := mustRandom(t, kekLen)
	_, priv, _ := newSlotKeypair()
	enc, err := sealPrivKey(kek, priv[:], privAAD("reader", ScopeRead))
	if err != nil {
		t.Fatal(err)
	}

	// Correct binding opens.
	if _, err := openPrivKey(kek, enc, privAAD("reader", ScopeRead)); err != nil {
		t.Fatalf("openPrivKey with correct AAD: %v", err)
	}
	// Scope flip read->write fails.
	if _, err := openPrivKey(kek, enc, privAAD("reader", ScopeWrite)); err != errSlotAuth {
		t.Errorf("scope flip error = %v, want errSlotAuth", err)
	}
	// Name change fails.
	if _, err := openPrivKey(kek, enc, privAAD("writer", ScopeRead)); err != errSlotAuth {
		t.Errorf("name change error = %v, want errSlotAuth", err)
	}
	// Wrong KEK (wrong passphrase) fails with the same error.
	if _, err := openPrivKey(mustRandom(t, kekLen), enc, privAAD("reader", ScopeRead)); err != errSlotAuth {
		t.Errorf("wrong-KEK error = %v, want errSlotAuth", err)
	}
}

// TestVerifierBindsScope confirms the verifier changes with Scope (so
// the trial loop rejects a scope-flipped slot before even opening the
// private key) but not with the namespace list.
func TestVerifierBindsScope(t *testing.T) {
	kek := mustRandom(t, kekLen)
	base := deriveVerifier(kek, "reader", ScopeRead)

	if !verifyMatches(base, deriveVerifier(kek, "reader", ScopeRead)) {
		t.Error("verifier should be deterministic")
	}
	if verifyMatches(base, deriveVerifier(kek, "reader", ScopeWrite)) {
		t.Error("verifier must change when scope changes")
	}
	if verifyMatches(base, deriveVerifier(kek, "writer", ScopeRead)) {
		t.Error("verifier must change when the name changes")
	}
	if verifyMatches(base, deriveVerifier(mustRandom(t, kekLen), "reader", ScopeRead)) {
		t.Error("verifier must change with the KEK")
	}
}

// TestPubMAC authenticates a slot public key; any field change flips it.
func TestPubMAC(t *testing.T) {
	ik := mustRandom(t, 32)
	pub := "cHVibGljLWtleQ==" // arbitrary base64
	base := derivePubMAC(ik, "reader", pub, ScopeRead, []string{"proj"})

	if base != derivePubMAC(ik, "reader", pub, ScopeRead, []string{"proj"}) {
		t.Error("pubMAC must be deterministic")
	}
	for _, mut := range []string{"name", "pub", "scope", "ns", "key"} {
		var got string
		switch mut {
		case "name":
			got = derivePubMAC(ik, "writer", pub, ScopeRead, []string{"proj"})
		case "pub":
			got = derivePubMAC(ik, "reader", "b3RoZXI=", ScopeRead, []string{"proj"})
		case "scope":
			got = derivePubMAC(ik, "reader", pub, ScopeWrite, []string{"proj"})
		case "ns":
			got = derivePubMAC(ik, "reader", pub, ScopeRead, []string{"prod"})
		case "key":
			got = derivePubMAC(mustRandom(t, 32), "reader", pub, ScopeRead, []string{"proj"})
		}
		if got == base {
			t.Errorf("pubMAC unchanged after mutating %q", mut)
		}
	}
}

// TestSlotKEKDeterministicAndDistinct confirms one scrypt master yields
// stable per-slot KEKs that differ by slot salt.
func TestSlotKEKDeterministicAndDistinct(t *testing.T) {
	master, err := vaultMasterKEK("pw", mustRandom(t, keyringSaltLen))
	if err != nil {
		t.Fatal(err)
	}
	saltA, saltB := mustRandom(t, 16), mustRandom(t, 16)
	a1, _ := slotKEK(master, saltA)
	a2, _ := slotKEK(master, saltA)
	b1, _ := slotKEK(master, saltB)
	if !bytes.Equal(a1, a2) {
		t.Error("slotKEK must be deterministic for the same salt")
	}
	if bytes.Equal(a1, b1) {
		t.Error("slotKEK must differ across slot salts")
	}
	if len(a1) != kekLen {
		t.Errorf("KEK len = %d, want %d", len(a1), kekLen)
	}
}

// TestSealedValueNDKID recovers the id without the key.
func TestSealedValueNDKID(t *testing.T) {
	ndk := mustRandom(t, ndkLen)
	id, _ := newNDKID()
	sealed, _ := sealValue(ndk, id, "proj", "proj:k", "v")
	got, err := sealedValueNDKID(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, id) {
		t.Errorf("ndkID = %x, want %x", got, id)
	}
}

// TestMACInputSeparators guards the 0x00 domain separation.
func TestMACInputSeparators(t *testing.T) {
	got := macInput("L", "a", "b")
	want := append([]byte("L"), 0x00)
	want = append(want, 'a', 0x00, 'b')
	if !bytes.Equal(got, want) {
		t.Errorf("macInput = %s, want %s", hex.EncodeToString(got), hex.EncodeToString(want))
	}
}
