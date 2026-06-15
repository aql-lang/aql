package vault

// integrity.go — tamper/change detection for vault.jsonic.
//
// Three layers (see the plan's Layered integrity section):
//
//  1. A keyless signature sidecar (vault.jsonic.sig): sha256 + generation
//     + size + mtime. Detects accidental edits and corruption on every
//     command; an attacker who rewrites the file can recompute the
//     sha256, so the sidecar alone is not tamper-evidence.
//  2. A keyed HMAC over (generation, sha256), verifiable by any session
//     that can reach the integrity key. It detects tampering by an
//     attacker with no valid passphrase. (See the note in keyslot.go on
//     why the integrity key is reachable by every slot rather than
//     admin-only: a non-admin write must still be able to maintain the
//     HMAC, and an authenticated insider is already constrained by the
//     per-slot scope/pubkey bindings, so the store HMAC is not the
//     control that stops them.)
//  3. An anti-rollback anchor: a monotonic high-water-mark generation
//     stored as a reserved keyring entry. A loaded generation below the
//     anchor is a rollback. This catches a partial rollback (vault.jsonic
//     rolled back while the keyring moved on). A fully-consistent
//     whole-folder rollback of a file-backend vault is a documented
//     residual risk — no trusted material lives outside the folder.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// StoreIntegrity classifies the on-disk state of vault.jsonic relative
// to its sidecar, the keyed HMAC, and the anti-rollback anchor.
type StoreIntegrity int

const (
	IntegrityOK             StoreIntegrity = iota // matches sidecar (and keyed HMAC, when checked)
	IntegrityUnverified                           // keyless match only; no integrity key to check the keyed layer
	IntegrityNoBaseline                           // no sidecar and no journal — a genuinely fresh vault
	IntegrityExternalChange                       // parses but doesn't match the sidecar or journal head
	IntegrityCorrupt                              // does not parse / truncated
	IntegrityHMACInvalid                          // keyed HMAC fails for the claimed (generation, sha256)
	IntegrityRollback                             // generation below the anti-rollback anchor
)

func (i StoreIntegrity) String() string {
	switch i {
	case IntegrityOK:
		return "ok"
	case IntegrityUnverified:
		return "unverified"
	case IntegrityNoBaseline:
		return "no-baseline"
	case IntegrityExternalChange:
		return "external-change"
	case IntegrityCorrupt:
		return "corrupt"
	case IntegrityHMACInvalid:
		return "hmac-invalid"
	case IntegrityRollback:
		return "rollback-detected"
	default:
		return "unknown"
	}
}

// maxQuarantine bounds the externally-modified store snapshots kept.
const maxQuarantine = 10

// storeSidecar is the keyless signature written beside vault.jsonic.
type storeSidecar struct {
	Generation int64  `json:"generation"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
	MtimeNs    int64  `json:"mtime_ns"`
	HMAC       string `json:"hmac,omitempty"` // keyed; empty when written without the integrity key
	// ContentHash is the sha256 of the store with Generation zeroed. It
	// lets SaveStore decide whether content actually changed (and so
	// whether to bump the generation) without the generation field
	// itself perturbing the comparison.
	ContentHash string `json:"content_hash,omitempty"`
}

// storeContentHash hashes a store with its Generation zeroed, so the
// generation counter (metadata about the content) does not perturb the
// content-change comparison.
func storeContentHash(s *Store) string {
	c := *s
	c.Generation = 0
	b, err := json.MarshalIndent(&c, "", "  ")
	if err != nil {
		return ""
	}
	return storeSHA256(b)
}

func sidecarPath(homeDir string) string { return StorePath(homeDir) + ".sig" }

func storeSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// storeHMAC computes the keyed integrity tag over (generation, sha256).
func storeHMAC(ik []byte, gen int64, sha string) string {
	m := hmac.New(sha256.New, ik)
	fmt.Fprintf(m, "%d\x00%s", gen, sha)
	return hex.EncodeToString(m.Sum(nil))
}

func readSidecar(homeDir string) (*storeSidecar, error) {
	data, err := os.ReadFile(sidecarPath(homeDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var sc storeSidecar
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("vault: parsing %s: %w", sidecarPath(homeDir), err)
	}
	return &sc, nil
}

// writeSidecar writes the signature for the just-written store bytes.
// hmac is included only when ik is non-nil.
func writeSidecar(homeDir string, gen int64, storeBytes, ik []byte) error {
	sha := storeSHA256(storeBytes)
	sc := storeSidecar{Generation: gen, SHA256: sha, Size: int64(len(storeBytes))}
	var parsed Store
	if json.Unmarshal(storeBytes, &parsed) == nil {
		sc.ContentHash = storeContentHash(&parsed)
	}
	if st, err := os.Stat(StorePath(homeDir)); err == nil {
		sc.MtimeNs = st.ModTime().UnixNano()
	}
	if ik != nil {
		sc.HMAC = storeHMAC(ik, gen, sha)
	}
	body, err := json.Marshal(&sc)
	if err != nil {
		return err
	}
	return writeFileAtomic(sidecarPath(homeDir), body, 0600)
}

// readAnchor returns the anti-rollback high-water-mark stored in the
// keyring, or 0 if unset.
func readAnchor(kr keyring) (int64, error) {
	v, err := kr.Get(anchorKeyName)
	if err == ErrNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, nil
	}
	return n, nil
}

// advanceAnchor raises the anti-rollback anchor to gen if it is higher.
func advanceAnchor(kr keyring, gen int64) error {
	cur, err := readAnchor(kr)
	if err != nil {
		return err
	}
	if gen <= cur {
		return nil
	}
	return kr.Set(anchorKeyName, strconv.FormatInt(gen, 10))
}

// quarantineStore copies the current (externally-modified) vault.jsonic
// to a snapshot named by its content hash, so the offending file is
// never lost when a mutation is blocked. Identical content dedupes to
// the same name; the ring is capped to maxQuarantine.
func quarantineStore(homeDir string) (string, error) {
	data, err := os.ReadFile(StorePath(homeDir))
	if err != nil {
		return "", err
	}
	dst := StorePath(homeDir) + ".external." + storeSHA256(data)[:12]
	if _, err := os.Stat(dst); err == nil {
		return dst, nil // dedupe: identical content already quarantined
	}
	if err := writeFileAtomic(dst, data, 0600); err != nil {
		return "", err
	}
	quarantinePrune(homeDir, maxQuarantine)
	return dst, nil
}

// storeGenerationOnDisk parses just the generation from the on-disk
// store, reporting whether the file parses at all.
func storeGenerationOnDisk(data []byte) (gen int64, parses bool) {
	var probe struct {
		Generation int64 `json:"generation"`
	}
	if json.Unmarshal(data, &probe) != nil {
		return 0, false
	}
	return probe.Generation, true
}

// sealIntegrity upgrades the keyless signature sidecar that SaveStore
// wrote into its keyed form — HMAC over (generation, sha256) under the
// session's integrity key — and advances the anti-rollback anchor to the
// current generation. It is a no-op for a legacy session (no integrity
// key) and is best-effort: a failure never fails the mutation. Call it
// after a successful envelope mutation. A mutation that does not seal
// simply leaves the keyless sidecar (reported as 'unverified' to the
// next keyed command), which the next sealed mutation re-establishes;
// the anchor only ever rises, so this degrades gracefully.
func sealIntegrity(homeDir string, sess *Session) {
	if sess == nil {
		return
	}
	ik := sess.IntegrityKey()
	if ik == nil {
		return
	}
	data, err := os.ReadFile(StorePath(homeDir))
	if err != nil {
		return
	}
	gen, ok := storeGenerationOnDisk(data)
	if !ok {
		return
	}
	_ = writeSidecar(homeDir, gen, data, ik)
	if kr := sess.Keyring(); kr != nil {
		_ = advanceAnchor(kr, gen)
	}
}

// checkStoreIntegrity classifies vault.jsonic against its sidecar, the
// keyed HMAC (when ik != nil), the journal head, and the anti-rollback
// anchor (anchorGen). It does not itself read the keyring; the caller
// supplies ik and anchorGen from the authenticated session.
func checkStoreIntegrity(homeDir string, ik []byte, anchorGen int64) (StoreIntegrity, error) {
	data, err := os.ReadFile(StorePath(homeDir))
	if err != nil {
		return IntegrityCorrupt, err
	}
	gen, parses := storeGenerationOnDisk(data)
	if !parses {
		return IntegrityCorrupt, nil
	}
	sha := storeSHA256(data)
	sc, err := readSidecar(homeDir)
	if err != nil {
		return IntegrityCorrupt, err
	}
	head, _ := journalHead(homeDir)

	// Rollback below the trusted anchor (only meaningful with a key).
	if ik != nil && anchorGen > 0 && gen < anchorGen {
		return IntegrityRollback, nil
	}

	if sc == nil {
		// No sidecar. Reconcile against the journal head if one exists,
		// so deleting the sidecar can't launder tampering.
		if head == nil {
			return IntegrityNoBaseline, nil
		}
		if sha != head.SHA256 {
			return IntegrityExternalChange, nil
		}
		if ik == nil {
			return IntegrityUnverified, nil
		}
		if head.HMAC == "" {
			return IntegrityUnverified, nil
		}
		want := journalChainHMAC(ik, head.Generation, head.SHA256, head.PrevHMAC)
		if !hmac.Equal([]byte(want), []byte(head.HMAC)) {
			return IntegrityHMACInvalid, nil
		}
		return IntegrityOK, nil
	}

	if sha != sc.SHA256 {
		return IntegrityExternalChange, nil
	}
	// Content matches the sidecar.
	if sc.HMAC == "" {
		// No keyed tag was written. For a no-key session that is simply
		// "ok" (legacy / no keyed layer expected); for a keyed session it
		// means the keyed layer has not been established yet (mutations
		// ran without the integrity key) — report honestly as unverified
		// rather than as tamper.
		if ik == nil {
			return IntegrityOK, nil
		}
		return IntegrityUnverified, nil
	}
	if ik == nil {
		return IntegrityUnverified, nil // keyed tag present, no key to check it
	}
	want := storeHMAC(ik, sc.Generation, sc.SHA256)
	if !hmac.Equal([]byte(want), []byte(sc.HMAC)) {
		return IntegrityHMACInvalid, nil
	}
	return IntegrityOK, nil
}
