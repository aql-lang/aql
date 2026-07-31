package vault

// keyslot.go — the envelope crypto behind scoped passwords.
//
// Layered key hierarchy (LUKS-keyslot style, asymmetric slots):
//
//	passphrase --scrypt(VaultSalt)--> masterKEK   (one scrypt per vault)
//	masterKEK  --HKDF(slot.Salt)----> slot KEK     (cheap, per slot)
//	slot KEK   --AES-GCM-----------> seals the slot's X25519 private key
//	slot X25519 keypair  ----------> seals/opens namespace data keys (NDK)
//	NDK        --AES-GCM-----------> seals/opens individual secret values
//
// One scrypt covers every slot, so authenticate is O(1) in slot count.
// Each value carries the id of the NDK that sealed it, so a namespace
// can hold two NDKs during a crash-safe rekey transition.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/nacl/box"
)

const (
	ndkLen      = 32 // namespace data key length (AES-256)
	ndkIDLen    = 8  // random per-NDK identifier
	kekLen      = 32 // AES-256 key-encryption key
	gcmNonceLen = 12 // AES-GCM nonce
	valueMagic  = "BORUE"
	valueFormat = 1

	// Domain-separation labels. Every keyed-MAC / AEAD-AAD input below is
	// the label followed by 0x00, then the fields joined with 0x00.
	// Scope/Namespaces are attacker-visible plaintext, so unseparated
	// concatenation is forbidden — the 0x00 boundaries are load-bearing.
	labelKEK      = "BORU-slot-kek" // HKDF info for slot KEK derivation
	labelVerifier = "BORU-verify"   // slot passphrase verifier
	labelPriv     = "BORUP"         // EncPrivKey AAD
	labelPubMAC   = "BORU-slot"     // public-key authentication MAC
)

// ErrNoKeyForValue is returned by openValue when the session holds no
// data key for the namespace/id that sealed the value (i.e. the
// namespace was not granted to this password).
var ErrNoKeyForValue = errors.New("vault: no data key for this namespace")

// newRandom returns n cryptographically random bytes.
func newRandom(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := randRead(b); err != nil {
		return nil, err
	}
	return b, nil
}

func newNDK() ([]byte, error)   { return newRandom(ndkLen) }
func newNDKID() ([]byte, error) { return newRandom(ndkIDLen) }

// zeroize overwrites b in place. Used to wipe KEKs, private keys, and
// unsealed NDKs once a Session is done with them.
func zeroize(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// vaultMasterKEK derives the one-per-vault master key via the existing
// scrypt KDF over the shared VaultSalt. This is the only scrypt call on
// the authentication path.
func vaultMasterKEK(passphrase string, vaultSalt []byte) ([]byte, error) {
	return p7scryptKey(passphrase, vaultSalt)
}

// slotKEK derives a per-slot key-encryption key from the master KEK and
// the slot's random HKDF info (its Salt field). HKDF is cheap, so this
// is O(1) per slot after the single scrypt.
func slotKEK(masterKEK, slotSalt []byte) ([]byte, error) {
	r := hkdf.New(sha256.New, masterKEK, slotSalt, []byte(labelKEK))
	out := make([]byte, kekLen)
	if _, err := p7ioReadFull(r, out); err != nil {
		return nil, err
	}
	return out, nil
}

// nsCanon returns the canonical, separator-joined form of a namespace
// allow-list for inclusion in a MAC/AAD: sorted and 0x00-joined.
// Namespace names cannot contain 0x00 (validNamespaceName forbids it),
// so this is unambiguous.
func nsCanon(namespaces []string) string {
	c := append([]string(nil), namespaces...)
	sort.Strings(c)
	return strings.Join(c, "\x00")
}

// macInput builds a domain-separated MAC/AAD message: label, 0x00, then
// each field separated by 0x00.
func macInput(label string, fields ...string) []byte {
	b := make([]byte, 0, len(label)+1+len(fields)*8)
	b = append(b, label...)
	b = append(b, 0x00)
	for i, f := range fields {
		if i > 0 {
			b = append(b, 0x00)
		}
		b = append(b, f...)
	}
	return b
}

// deriveVerifier computes the slot passphrase verifier, binding Name and
// Scope so flipping the plaintext Scope (a pure-policy tier with no
// crypto gate of its own) fails authentication. Namespaces are NOT bound
// here: they are gated cryptographically by NDK possession (editing the
// Namespaces field grants nothing without the wrapped data key), and
// leaving them out lets an admin reassign namespaces — re-wrapping NDKs
// to the slot's public key — without the holder's passphrase.
func deriveVerifier(kek []byte, name, scope string) string {
	m := hmac.New(sha256.New, kek)
	m.Write(macInput(labelVerifier, name, scope))
	return hex.EncodeToString(m.Sum(nil))
}

// verifyMatches constant-time compares a presented verifier hex against
// the stored one.
func verifyMatches(stored, presented string) bool {
	return hmac.Equal([]byte(stored), []byte(presented))
}

// derivePubMAC authenticates a slot's public key against tampering using
// the admin-held integrity key. add/assign/rm verify this before
// unsealing or re-sealing any NDK to PublicKey.
func derivePubMAC(integrityKey []byte, name, pubB64, scope string, namespaces []string) string {
	m := hmac.New(sha256.New, integrityKey)
	m.Write(macInput(labelPubMAC, name, pubB64, scope, nsCanon(namespaces)))
	return hex.EncodeToString(m.Sum(nil))
}

// privAAD builds the AES-GCM additional data binding an EncPrivKey to its
// slot Name and Scope — and, for a temporary password, its ExpiresAt — so
// a naive plaintext edit of any of them fails authentication (the AEAD tag
// no longer verifies). A permanent slot (empty ExpiresAt) yields the
// pre-v6 AAD unchanged, so existing slots still open. Namespaces are gated
// by NDK possession, not bound here (see deriveVerifier).
//
// NOTE: a slot HOLDER possesses the KEK, private key, and integrity key,
// so a determined holder with write access to the store can re-seal a slot
// under a new AAD and extend their own expiry. This binding stops casual
// store edits; the hard revoke is an admin `password rm` / `rm --temp`.
func privAAD(name, scope, expiresAt string) []byte {
	if expiresAt == "" {
		return macInput(labelPriv, name, scope)
	}
	return macInput(labelPriv, name, scope, expiresAt)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	blk, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(blk)
}

// newSlotKeypair generates an X25519 keypair for a new slot.
func newSlotKeypair() (pub, priv *[32]byte, err error) {
	return box.GenerateKey(p7boxRand)
}

// sealPrivKey AES-GCM-seals a slot's X25519 private key under its KEK,
// returning base64(nonce|ciphertext|tag).
func sealPrivKey(kek, priv, aad []byte) (string, error) {
	g, err := newGCM(kek)
	if err != nil {
		return "", err
	}
	nonce, err := newRandom(gcmNonceLen)
	if err != nil {
		return "", err
	}
	ct := g.Seal(nil, nonce, priv, aad)
	return base64.StdEncoding.EncodeToString(append(nonce, ct...)), nil
}

// openPrivKey decrypts a slot's X25519 private key. A wrong passphrase
// (wrong KEK) or a tampered Scope/Namespaces (wrong AAD) fails here.
func openPrivKey(kek []byte, encB64 string, aad []byte) (*[32]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encB64)
	if err != nil {
		return nil, err
	}
	if len(raw) < gcmNonceLen+16 {
		return nil, errors.New("vault: enc_priv_key truncated")
	}
	g, err := newGCM(kek)
	if err != nil {
		return nil, err
	}
	pt, err := g.Open(nil, raw[:gcmNonceLen], raw[gcmNonceLen:], aad)
	if err != nil {
		return nil, errSlotAuth
	}
	if len(pt) != 32 {
		return nil, errors.New("vault: private key wrong length")
	}
	var k [32]byte
	copy(k[:], pt)
	zeroize(pt)
	return &k, nil
}

// errSlotAuth is the single non-revealing error for a failed slot
// unlock (wrong passphrase, or tampered scope/namespace binding). It
// must not distinguish the two, mirroring aesGCMOpen.
var errSlotAuth = errors.New("vault: wrong passphrase or corrupt keyring")

// sealNDK seals a namespace data key to a slot's public key using a NaCl
// anonymous sealed box (ephemeral-key, self-contained), base64-encoded.
func sealNDK(ndk []byte, slotPub *[32]byte) (string, error) {
	sealed, err := box.SealAnonymous(nil, ndk, slotPub, p7boxRand)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// openNDK opens a sealed NDK with the slot's keypair.
func openNDK(sealedB64 string, slotPub, slotPriv *[32]byte) ([]byte, error) {
	sealed, err := base64.StdEncoding.DecodeString(sealedB64)
	if err != nil {
		return nil, err
	}
	ndk, ok := box.OpenAnonymous(nil, sealed, slotPub, slotPriv)
	if !ok {
		return nil, errors.New("vault: cannot open wrapped namespace key")
	}
	return ndk, nil
}

// valueAAD binds a sealed value to its NDK id, namespace, and alias so a
// ciphertext cannot be relabeled across any of them.
func valueAAD(ndkID []byte, namespace, alias string) []byte {
	b := make([]byte, 0, len(valueMagic)+1+ndkIDLen+len(namespace)+1+len(alias))
	b = append(b, valueMagic...)
	b = append(b, byte(valueFormat))
	b = append(b, ndkID...)
	b = append(b, namespace...)
	b = append(b, 0x00)
	b = append(b, alias...)
	return b
}

// sealValue seals a secret value under an NDK, producing the
// self-describing base64 envelope the backend stores:
//
//	base64( "BORUE" | format(1) | ndkID(8) | nonce(12) | ciphertext|tag )
func sealValue(ndk, ndkID []byte, namespace, alias, plaintext string) (string, error) {
	g, err := newGCM(ndk)
	if err != nil {
		return "", err
	}
	nonce, err := newRandom(gcmNonceLen)
	if err != nil {
		return "", err
	}
	ct := g.Seal(nil, nonce, []byte(plaintext), valueAAD(ndkID, namespace, alias))
	out := make([]byte, 0, len(valueMagic)+1+ndkIDLen+gcmNonceLen+len(ct))
	out = append(out, valueMagic...)
	out = append(out, byte(valueFormat))
	out = append(out, ndkID...)
	out = append(out, nonce...)
	out = append(out, ct...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// sealedValueNDKID extracts the NDK id from a sealed value without
// decrypting it (used to pick which NDK opens it).
func sealedValueNDKID(sealedB64 string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(sealedB64)
	if err != nil {
		return nil, err
	}
	hdr := len(valueMagic) + 1 + ndkIDLen
	if len(raw) < hdr+gcmNonceLen+16 || string(raw[:len(valueMagic)]) != valueMagic {
		return nil, errors.New("vault: not a BORUE sealed value")
	}
	id := make([]byte, ndkIDLen)
	copy(id, raw[len(valueMagic)+1:hdr])
	return id, nil
}

// openValue decrypts a sealed value. ndkByID resolves an NDK id to its
// 32-byte key; if it returns false the session lacks that namespace's
// key and ErrNoKeyForValue is returned.
func openValue(sealedB64, namespace, alias string, ndkByID func(id []byte) ([]byte, bool)) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(sealedB64)
	if err != nil {
		return "", err
	}
	hdr := len(valueMagic) + 1 + ndkIDLen
	if len(raw) < hdr+gcmNonceLen+16 {
		return "", errors.New("vault: sealed value truncated")
	}
	if string(raw[:len(valueMagic)]) != valueMagic {
		return "", errors.New("vault: not a BORUE sealed value")
	}
	if f := raw[len(valueMagic)]; f != valueFormat {
		return "", fmt.Errorf("vault: sealed value format %d unsupported", f)
	}
	ndkID := raw[len(valueMagic)+1 : hdr]
	nonce := raw[hdr : hdr+gcmNonceLen]
	ct := raw[hdr+gcmNonceLen:]
	ndk, ok := ndkByID(ndkID)
	if !ok {
		return "", ErrNoKeyForValue
	}
	g, err := newGCM(ndk)
	if err != nil {
		return "", err
	}
	pt, err := g.Open(nil, nonce, ct, valueAAD(ndkID, namespace, alias))
	if err != nil {
		return "", errors.New("vault: cannot decrypt value (wrong key or corrupt entry)")
	}
	return string(pt), nil
}
