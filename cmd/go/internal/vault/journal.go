package vault

// journal.go — the event-sourced store history (vault.jsonic.log).
//
// One append-only, hash-chained record per committed generation. Each
// record carries a redacted snapshot of the Store (no slot crypto
// material, no Config) so the journal is browsable history and the
// restore backbone, without being a permanent offline passphrase oracle
// or a Config-secret archive.
//
// The chain HMAC binds each record to its predecessor, so truncation or
// splicing of the log is detectable by an admin/keyed verifier. Records
// written without the integrity key (legacy / no-key paths) carry an
// empty HMAC and break the chain there — verifyJournalChain reports the
// first such gap rather than silently accepting it.

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// defaultJournalKeep is the number of newest generations retained when
// no journal.keep config override is set.
const defaultJournalKeep = 50

// journalRecord is one committed generation in vault.jsonic.log.
type journalRecord struct {
	Generation int64  `json:"generation"`
	Timestamp  string `json:"ts"`
	Actor      string `json:"actor,omitempty"`
	Action     string `json:"action,omitempty"`
	SHA256     string `json:"sha256"`
	PrevHMAC   string `json:"prev_hmac,omitempty"`
	HMAC       string `json:"hmac,omitempty"`
	// Store is the redacted post-mutation snapshot (slot crypto material
	// zeroed, Config omitted). It is enough to display history and to
	// reconstruct non-secret slot metadata on restore.
	Store *Store `json:"store,omitempty"`
}

func journalPath(homeDir string) string { return StorePath(homeDir) + ".log" }

// redactStore returns a deep-enough copy of s with everything that must
// not be archived in cleartext stripped: each slot's Salt/Verifier/
// PubMAC/EncPrivKey/WrappedKeys are zeroed, VaultSalt is dropped, and
// Config is omitted. Non-secret slot metadata (Name/Scope/Namespaces/
// PublicKey/timestamps) is kept for history and restore.
func redactStore(s *Store) *Store {
	c := *s
	c.Config = nil
	c.VaultSalt = ""
	c.CurrentNDK = nil // ns->id map, not needed for history/restore
	// Deep-copy the slices we keep so a journal snapshot never shares
	// backing with the live store (defensive against future async use).
	c.Aliases = append([]Alias(nil), s.Aliases...)
	c.Capabilities = append([]Capability(nil), s.Capabilities...)
	c.Agents = append([]string(nil), s.Agents...)
	if len(s.Passwords) > 0 {
		ps := make([]PasswordSlot, len(s.Passwords))
		for i, p := range s.Passwords {
			p.Salt = ""
			p.Verifier = ""
			p.PubMAC = ""
			p.EncPrivKey = ""
			p.WrappedKeys = nil
			ps[i] = p
		}
		c.Passwords = ps
	} else {
		c.Passwords = nil
	}
	return &c
}

// journalChainHMAC computes the record HMAC binding (generation, sha256)
// to the previous record's HMAC.
func journalChainHMAC(ik []byte, gen int64, sha, prevHMAC string) string {
	m := hmac.New(sha256.New, ik)
	fmt.Fprintf(m, "%d\x00%s\x00%s", gen, sha, prevHMAC)
	return hex.EncodeToString(m.Sum(nil))
}

// nowRFC3339Nano is the timestamp form used in journal/audit records.
func nowRFC3339Nano() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// appendJournal appends one record for a committed generation. ik may be
// nil (no integrity key available); then the record's HMAC is empty and
// the chain is intentionally broken at this point.
func appendJournal(homeDir string, ik []byte, gen int64, actor, action, sha string, redacted *Store) error {
	prev := ""
	if head, err := journalHead(homeDir); err == nil && head != nil {
		prev = head.HMAC
	}
	rec := journalRecord{
		Generation: gen,
		Timestamp:  nowRFC3339Nano(),
		Actor:      actor,
		Action:     action,
		SHA256:     sha,
		PrevHMAC:   prev,
		Store:      redacted,
	}
	if ik != nil {
		rec.HMAC = journalChainHMAC(ik, gen, sha, prev)
	}
	line, err := jsonMarshal(&rec)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(vaultFolder(homeDir), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(journalPath(homeDir), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fileWrite(f, append(line, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

// readJournal parses every record in order. A missing log is (nil, nil).
func readJournal(homeDir string) ([]journalRecord, error) {
	f, err := os.Open(journalPath(homeDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []journalRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec journalRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("vault: corrupt journal record: %w", err)
		}
		out = append(out, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// journalHead returns the newest record, or (nil, nil) if the log is
// empty/absent.
func journalHead(homeDir string) (*journalRecord, error) {
	recs, err := readJournal(homeDir)
	if err != nil || len(recs) == 0 {
		return nil, err
	}
	return &recs[len(recs)-1], nil
}

// findJournalGeneration returns the record for a specific generation, or
// (nil, nil) if absent.
func findJournalGeneration(homeDir string, gen int64) (*journalRecord, error) {
	recs, err := readJournal(homeDir)
	if err != nil {
		return nil, err
	}
	for i := len(recs) - 1; i >= 0; i-- {
		if recs[i].Generation == gen {
			return &recs[i], nil
		}
	}
	return nil, nil
}

// pruneJournal rewrites the log keeping only the newest keep records.
func pruneJournal(homeDir string, keep int) error {
	if keep <= 0 {
		keep = defaultJournalKeep
	}
	recs, err := readJournal(homeDir)
	if err != nil {
		return err
	}
	if len(recs) <= keep {
		return nil
	}
	recs = recs[len(recs)-keep:]
	var buf strings.Builder
	for i := range recs {
		line, err := jsonMarshal(&recs[i])
		if err != nil {
			return err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return writeFileAtomic(journalPath(homeDir), []byte(buf.String()), 0600)
}

// verifyJournalChain walks the chain from the oldest retained record,
// confirming each record's HMAC over (generation, sha256, prev_hmac).
// It returns ok=false at the first record whose HMAC is missing or does
// not verify, with badGen identifying it. A chain that begins after a
// prune still verifies internally (prev of the first retained record is
// taken as given), so this detects tampering/splicing within the
// retained window.
func verifyJournalChain(homeDir string, ik []byte) (ok bool, badGen int64, err error) {
	recs, err := readJournal(homeDir)
	if err != nil {
		return false, 0, err
	}
	if ik == nil {
		return false, 0, fmt.Errorf("vault: cannot verify journal chain without the integrity key")
	}
	prev := ""
	started := false
	for i := range recs {
		r := &recs[i]
		if r.HMAC == "" {
			return false, r.Generation, nil
		}
		if started && r.PrevHMAC != prev {
			return false, r.Generation, nil
		}
		want := journalChainHMAC(ik, r.Generation, r.SHA256, r.PrevHMAC)
		if !hmac.Equal([]byte(want), []byte(r.HMAC)) {
			return false, r.Generation, nil
		}
		prev = r.HMAC
		started = true
	}
	return true, 0, nil
}

// journalKeepFromConfig reads the journal.keep retention from store
// config, defaulting to defaultJournalKeep.
func journalKeepFromConfig(s *Store) int {
	if s == nil || s.Config == nil {
		return defaultJournalKeep
	}
	if v, ok := s.Config["journal.keep"]; ok {
		switch n := v.(type) {
		case float64:
			if int(n) > 0 {
				return int(n)
			}
		case int:
			if n > 0 {
				return n
			}
		case string:
			var parsed int
			if _, err := fmt.Sscanf(n, "%d", &parsed); err == nil && parsed > 0 {
				return parsed
			}
		}
	}
	return defaultJournalKeep
}

// --- history / restore CLI -------------------------------------------------

// runHistory implements `boru vault history [--limit=N]` — the
// event-sourced generation timeline from vault.jsonic.log.
func runHistory(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault history", flag.ContinueOnError)
	fs.SetOutput(stderr)
	limit := fs.Int("limit", 0, "show only the newest N generations (0 = all)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if _, err := requireStore(homeDir); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	recs, err := readJournal(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if len(recs) == 0 {
		fmt.Fprintln(stdout, "(no history recorded)")
		return 0
	}
	start := 0
	if *limit > 0 && len(recs) > *limit {
		start = len(recs) - *limit
	}
	printHistory(stdout, recs[start:])
	return 0
}

func printHistory(stdout io.Writer, recs []journalRecord) {
	fmt.Fprintf(stdout, "%-6s %-30s %-18s %-18s %s\n", "GEN", "TIMESTAMP", "ACTOR", "ACTION", "CONTENTS")
	for _, r := range recs {
		na, ns, nc := 0, 0, 0
		if r.Store != nil {
			na = len(r.Store.Aliases)
			ns = len(r.Store.Passwords)
			nc = len(r.Store.Capabilities)
		}
		fmt.Fprintf(stdout, "%-6d %-30s %-18s %-18s %d aliases, %d caps, %d slots\n",
			r.Generation, r.Timestamp, dash(r.Actor), dash(r.Action), na, nc, ns)
	}
}

// runRestore implements `boru vault restore`. Without --rekey-style
// access to slot crypto (the journal is redacted), it restores the
// metadata layer — aliases, agents, lock state, and a monotonic
// capability merge — from a past generation, while PRESERVING the
// current password slots, namespace keys, Config, and keyring values.
// It is admin-gated and tier-2 confirmed.
func runRestore(args []string, homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault restore", flag.ContinueOnError)
	fs.SetOutput(stderr)
	list := fs.Bool("list", false, "list restorable generations")
	gen := fs.Int64("generation", 0, "generation to restore (see vault history)")
	dryRun := fs.Bool("dry-run", false, "report what would change without writing")
	yes := fs.Bool("yes", false, "confirm the restore non-interactively")
	fs.BoolVar(yes, "y", false, "confirm the restore non-interactively (shorthand)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	sess, err := authenticate(s, homeDir, stdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer sess.Close()
	if err := requireScope(sess, OpAdmin); err != nil {
		fmt.Fprintln(stderr, "error: only an admin password can restore the vault")
		return 1
	}

	recs, err := readJournal(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if *list {
		if len(recs) == 0 {
			fmt.Fprintln(stdout, "(no restorable generations)")
			return 0
		}
		printHistory(stdout, recs)
		return 0
	}
	if *gen <= 0 {
		fmt.Fprintln(stderr, "error: specify --generation=G (see `boru vault history`) or --list")
		return 1
	}
	rec, err := c7findJournalGeneration(homeDir, *gen)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if rec == nil || rec.Store == nil {
		fmt.Fprintf(stderr, "error: no journal snapshot for generation %d\n", *gen)
		return 1
	}
	target := rec.Store

	if *dryRun {
		fmt.Fprintf(stdout, "dry-run: restore to generation %d would set aliases %d->%d, agents %d->%d, locked=%t,\n",
			*gen, len(s.Aliases), len(target.Aliases), len(s.Agents), len(target.Agents), target.Locked)
		fmt.Fprintf(stdout, "  and merge capabilities (current revocations and quota meters preserved).\n")
		fmt.Fprintln(stdout, "  Preserved (NOT rolled back): password slots, vault salt, namespace keys, Config, keyring values.")
		return 0
	}

	if err := confirmDestructive(confirmTier2, fmt.Sprintf("%d", *gen),
		fmt.Sprintf("restore vault metadata to generation %d", *gen), *yes, true, stdin, stdout); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	if err := withVaultLock(homeDir, func() error {
		cur, err := c7loadStore(homeDir)
		if err != nil {
			return err
		}
		if cur == nil {
			return fmt.Errorf("vault not initialized")
		}
		cur.Aliases = target.Aliases
		cur.Agents = target.Agents
		cur.Locked = target.Locked
		cur.Capabilities = mergeCapabilities(cur.Capabilities, target.Capabilities)
		return SaveStore(homeDir, cur)
	}); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_ = appendAudit(homeDir, AuditEvent{Action: "vault.restore", Outcome: "ok", Reason: fmt.Sprintf("from-generation=%d", *gen)})
	fmt.Fprintf(stdout, "restored vault metadata from generation %d (committed as a new generation)\n", *gen)
	fmt.Fprintln(stderr, "note: password slots, namespace keys, Config, and keyring secret values were preserved, not rolled back")
	return 0
}

// mergeCapabilities reinstates the target generation's capabilities but
// keeps the current generation monotonic: a capability revoked now stays
// revoked, and quota meters never roll backward. A capability that was
// added after the target generation and has since been revoked is kept
// (revoked) rather than resurrected with access.
func mergeCapabilities(cur, target []Capability) []Capability {
	byID := map[string]Capability{}
	for _, c := range target {
		byID[c.ID] = c
	}
	for _, c := range cur {
		if t, ok := byID[c.ID]; ok {
			if c.Revoked {
				t.Revoked = true
			}
			if c.UsedCalls > t.UsedCalls {
				t.UsedCalls = c.UsedCalls
			}
			if c.UsedCostCents > t.UsedCostCents {
				t.UsedCostCents = c.UsedCostCents
			}
			byID[c.ID] = t
		} else if c.Revoked {
			byID[c.ID] = c
		}
	}
	out := make([]Capability, 0, len(byID))
	for _, c := range byID {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// quarantineGlob matches the externally-modified store snapshots.
func quarantineGlob(homeDir string) string {
	return StorePath(homeDir) + ".external.*"
}

// quarantinePrune keeps only the newest keep quarantine snapshots.
func quarantinePrune(homeDir string, keep int) {
	matches, err := filepath.Glob(quarantineGlob(homeDir))
	if err != nil || len(matches) <= keep {
		return
	}
	type fi struct {
		path string
		mod  time.Time
	}
	infos := make([]fi, 0, len(matches))
	for _, m := range matches {
		st, err := os.Stat(m)
		if err != nil {
			continue
		}
		infos = append(infos, fi{m, st.ModTime()})
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].mod.After(infos[j].mod) })
	for _, x := range infos[min(keep, len(infos)):] {
		_ = os.Remove(x.path)
	}
}
