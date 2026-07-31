package vault

// keyslot_seam7_test.go — seam7 coverage for the envelope-crypto error
// arms in keyslot.go. Most are reached by direct calls with malformed
// inputs; the entropy/KDF arms swap the randRead / p7boxRand / p7ioReadFull
// seams (saved and restored with t.Cleanup, no t.Parallel).

import (
	"encoding/base64"
	"errors"
	"io"
	"testing"
)

// --- shared p7 seam-swap helpers (used across the three seam7 files) -------

// p7boom is the canonical failure returned by the p7 seam stubs.
func p7boom() error { return errors.New("p7 seam boom") }

// p7failRead is a randRead seam that always fails (entropy exhausted).
func p7failRead([]byte) (int, error) { return 0, p7boom() }

// p7countRead returns a randRead seam that fills deterministically for the
// first ok calls and then fails on every later call. Used to let an early
// entropy draw succeed while a specific later one fails.
func p7countRead(ok int) func([]byte) (int, error) {
	n := 0
	return func(b []byte) (int, error) {
		n++
		if n > ok {
			return 0, p7boom()
		}
		for i := range b {
			b[i] = 0x7
		}
		return len(b), nil
	}
}

// p7failReader is an io.Reader (for p7boxRand) that always fails.
type p7failReader struct{}

func (p7failReader) Read([]byte) (int, error) { return 0, p7boom() }

// p7byteReader is an io.Reader (for p7boxRand) yielding `left` bytes total
// then failing, so an early box.GenerateKey succeeds and a later draw fails.
type p7byteReader struct{ left int }

func (r *p7byteReader) Read(b []byte) (int, error) {
	if r.left <= 0 {
		return 0, p7boom()
	}
	n := len(b)
	if n > r.left {
		n = r.left
	}
	for i := 0; i < n; i++ {
		b[i] = 0x7
	}
	r.left -= n
	return n, nil
}

// p7swapRandRead swaps the randRead seam and restores it on cleanup.
func p7swapRandRead(t *testing.T, fn func([]byte) (int, error)) {
	t.Helper()
	old := randRead
	randRead = fn
	t.Cleanup(func() { randRead = old })
}

// p7swapBoxRand swaps the p7boxRand seam and restores it on cleanup.
func p7swapBoxRand(t *testing.T, r io.Reader) {
	t.Helper()
	old := p7boxRand
	p7boxRand = r
	t.Cleanup(func() { p7boxRand = old })
}

// p7swapIOReadFull swaps the p7ioReadFull seam and restores it on cleanup.
func p7swapIOReadFull(t *testing.T, fn func(io.Reader, []byte) (int, error)) {
	t.Helper()
	old := p7ioReadFull
	p7ioReadFull = fn
	t.Cleanup(func() { p7ioReadFull = old })
}

// p7swapScryptKey swaps the p7scryptKey seam and restores it on cleanup.
func p7swapScryptKey(t *testing.T, fn func(string, []byte) ([]byte, error)) {
	t.Helper()
	old := p7scryptKey
	p7scryptKey = fn
	t.Cleanup(func() { p7scryptKey = old })
}

// p7validKEK is a 32-byte key that aes.NewCipher accepts (AES-256).
func p7validKEK() []byte {
	k := make([]byte, kekLen)
	for i := range k {
		k[i] = byte(i + 1)
	}
	return k
}

// --- newRandom / slotKEK entropy+KDF arms ----------------------------------

func TestP7_NewRandomEntropyFailure(t *testing.T) {
	p7swapRandRead(t, p7failRead)
	if _, err := newRandom(keyringSaltLen); err == nil {
		t.Fatal("newRandom should surface a randRead failure")
	}
	// Positive: with the real seam it succeeds and yields n bytes.
	randRead = func(b []byte) (int, error) { return len(b), nil }
	b, err := newRandom(ndkLen)
	if err != nil || len(b) != ndkLen {
		t.Fatalf("newRandom real = %v, len %d", err, len(b))
	}
}

func TestP7_SlotKEKReadFullFailure(t *testing.T) {
	p7swapIOReadFull(t, func(io.Reader, []byte) (int, error) { return 0, p7boom() })
	if _, err := slotKEK(p7validKEK(), []byte("salt")); err == nil {
		t.Fatal("slotKEK should surface an io.ReadFull failure")
	}
}

// --- newGCM / sealPrivKey / openPrivKey ------------------------------------

func TestP7_NewGCMBadKeyLength(t *testing.T) {
	if _, err := newGCM([]byte{1, 2, 3}); err == nil {
		t.Fatal("newGCM with a 3-byte key must fail aes.NewCipher")
	}
	if _, err := newGCM(p7validKEK()); err != nil {
		t.Fatalf("newGCM with a 32-byte key = %v", err)
	}
}

func TestP7_SealPrivKeyArms(t *testing.T) {
	aad := privAAD("n", ScopeRead, "")
	// bad KEK length -> newGCM error arm.
	if _, err := sealPrivKey([]byte{1, 2, 3}, make([]byte, 32), aad); err == nil {
		t.Fatal("sealPrivKey with a bad KEK should fail")
	}
	// valid KEK but entropy failure -> newRandom(nonce) error arm.
	p7swapRandRead(t, p7failRead)
	if _, err := sealPrivKey(p7validKEK(), make([]byte, 32), aad); err == nil {
		t.Fatal("sealPrivKey should surface a nonce entropy failure")
	}
}

func TestP7_OpenPrivKeyArms(t *testing.T) {
	kek := p7validKEK()
	aad := privAAD("n", ScopeRead, "")
	// invalid base64.
	if _, err := openPrivKey(kek, "!!! not base64 !!!", aad); err == nil {
		t.Fatal("openPrivKey should reject invalid base64")
	}
	// decodes but shorter than nonce+tag.
	short := base64.StdEncoding.EncodeToString(make([]byte, 10))
	if _, err := openPrivKey(kek, short, aad); err == nil {
		t.Fatal("openPrivKey should reject a truncated blob")
	}
	// long enough, but a bad KEK fails newGCM.
	long := base64.StdEncoding.EncodeToString(make([]byte, 30))
	if _, err := openPrivKey([]byte{1, 2, 3}, long, aad); err == nil {
		t.Fatal("openPrivKey with a bad KEK should fail newGCM")
	}
	// decrypts cleanly to a non-32-byte plaintext -> wrong-length arm.
	g, err := newGCM(kek)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcmNonceLen)
	ct := g.Seal(nil, nonce, []byte("short"), aad)
	enc := base64.StdEncoding.EncodeToString(append(append([]byte{}, nonce...), ct...))
	if _, err := openPrivKey(kek, enc, aad); err == nil {
		t.Fatal("openPrivKey should reject a non-32-byte plaintext")
	}
}

// --- sealNDK / openNDK -----------------------------------------------------

func TestP7_SealNDKBoxFailure(t *testing.T) {
	pub, _, err := newSlotKeypair() // real entropy first
	if err != nil {
		t.Fatal(err)
	}
	p7swapBoxRand(t, p7failReader{})
	if _, err := sealNDK(make([]byte, ndkLen), pub); err == nil {
		t.Fatal("sealNDK should surface a box.SealAnonymous failure")
	}
}

func TestP7_OpenNDKBadBase64(t *testing.T) {
	pub, priv, err := newSlotKeypair()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openNDK("!!! not base64 !!!", pub, priv); err == nil {
		t.Fatal("openNDK should reject invalid base64")
	}
}

// --- sealValue / sealedValueNDKID / openValue ------------------------------

func TestP7_SealValueArms(t *testing.T) {
	id := make([]byte, ndkIDLen)
	// bad NDK length -> newGCM arm.
	if _, err := sealValue([]byte{1, 2, 3}, id, "ns", "alias", "x"); err == nil {
		t.Fatal("sealValue with a bad NDK should fail newGCM")
	}
	// valid NDK but entropy failure -> newRandom(nonce) arm.
	p7swapRandRead(t, p7failRead)
	if _, err := sealValue(make([]byte, ndkLen), id, "ns", "alias", "x"); err == nil {
		t.Fatal("sealValue should surface a nonce entropy failure")
	}
}

func TestP7_SealedValueNDKIDArms(t *testing.T) {
	if _, err := sealedValueNDKID("!!! not base64 !!!"); err == nil {
		t.Fatal("sealedValueNDKID should reject invalid base64")
	}
	short := base64.StdEncoding.EncodeToString(make([]byte, 20))
	if _, err := sealedValueNDKID(short); err == nil {
		t.Fatal("sealedValueNDKID should reject a too-short value")
	}
}

func TestP7_OpenValueArms(t *testing.T) {
	ndkByID := func([]byte) ([]byte, bool) { return make([]byte, ndkLen), true }
	// invalid base64.
	if _, err := openValue("!!! not base64 !!!", "ns", "a", ndkByID); err == nil {
		t.Fatal("openValue should reject invalid base64")
	}
	// decodes but truncated.
	trunc := base64.StdEncoding.EncodeToString(make([]byte, 20))
	if _, err := openValue(trunc, "ns", "a", ndkByID); err == nil {
		t.Fatal("openValue should reject a truncated value")
	}
	// long enough but wrong magic.
	wrongMagic := base64.StdEncoding.EncodeToString(make([]byte, 45))
	if _, err := openValue(wrongMagic, "ns", "a", ndkByID); err == nil {
		t.Fatal("openValue should reject a non-BORUE value")
	}
	// correct magic, unsupported format byte.
	badFmt := make([]byte, 45)
	copy(badFmt, valueMagic)
	badFmt[len(valueMagic)] = byte(valueFormat + 9)
	if _, err := openValue(base64.StdEncoding.EncodeToString(badFmt), "ns", "a", ndkByID); err == nil {
		t.Fatal("openValue should reject an unsupported format byte")
	}
	// valid header, but ndkByID hands back a bad-length key -> newGCM arm.
	// Size it from the constants so a change to valueMagic cannot silently
	// shrink this below the length gate and skip the arm under test.
	hdr := make([]byte, len(valueMagic)+1+ndkIDLen+gcmNonceLen+16)
	copy(hdr, valueMagic)
	hdr[len(valueMagic)] = byte(valueFormat)
	badKey := func([]byte) ([]byte, bool) { return []byte{1, 2, 3}, true }
	if _, err := openValue(base64.StdEncoding.EncodeToString(hdr), "ns", "a", badKey); err == nil {
		t.Fatal("openValue should fail newGCM for a bad-length data key")
	}
}
