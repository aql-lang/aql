package vault

// password.go — `aql vault password <add|assign|set|rm|list>`: scoped
// named passwords (keyslots) over the envelope (keyslot.go / session.go).
//
// The first `password add` on a legacy single-passphrase vault migrates
// it to the envelope format: it generates the per-namespace data keys,
// turns the current passphrase into a seed "admin" slot, re-seals every
// secret under its namespace key, and adds the requested new slot. From
// then on the vault is envelope-mode and authenticate() matches a
// supplied passphrase to a slot.

import (
	"crypto/hmac"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/aql-lang/aql/cmd/go/internal/auth"
)

// EnvNewPassphrase supplies the NEW password for `password add`/`set` in
// non-interactive use (AQL_VAULT_PASSPHRASE remains the authenticating
// passphrase).
const EnvNewPassphrase = "AQL_VAULT_NEW_PASSPHRASE"

// runPassword dispatches the password sub-verbs.
func runPassword(args []string, homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "error: usage: aql vault password <add|assign|set|rm|list> [args...]")
		return 1
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "add":
		return runPasswordAdd(rest, homeDir, stdin, stdout, stderr)
	case "list", "ls":
		return runPasswordList(rest, homeDir, stdout, stderr)
	case "rm", "remove":
		return runPasswordRm(rest, homeDir, stdin, stdout, stderr)
	case "assign":
		return runPasswordAssign(rest, homeDir, stdin, stdout, stderr)
	case "set":
		return runPasswordSet(rest, homeDir, stdin, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "error: unknown password verb %q (expected add, assign, set, rm, or list)\n", sub)
		return 1
	}
}

// --- list ------------------------------------------------------------------

func runPasswordList(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault password list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if !s.HasPasswordSlots() {
		fmt.Fprintln(stdout, "no password slots (legacy single-passphrase vault)")
		return 0
	}
	fmt.Fprintf(stdout, "%-16s %-7s %-24s %s\n", "NAME", "SCOPE", "NAMESPACES", "CREATED")
	for _, p := range s.SortedPasswordSlots() {
		fmt.Fprintf(stdout, "%-16s %-7s %-24s %s\n", p.Name, p.Scope, namespacesLabel(p.Namespaces), p.CreatedAt)
	}
	return 0
}

// namespacesLabel renders a slot's namespace allow-list for display.
func namespacesLabel(ns []string) string {
	if len(ns) == 0 {
		return "*"
	}
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = nsLabel(n)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

// --- add -------------------------------------------------------------------

func runPasswordAdd(args []string, homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault password add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	scope := fs.String("scope", ScopeRead, "scope: read, write, move, or admin")
	nsFlag := fs.String("namespaces", "*", "comma-separated namespaces this password may access ('*' = all, ':' = root)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "error: usage: aql vault password add [--scope=...] [--namespaces=...] <name>")
		return 1
	}
	name := fs.Arg(0)
	if !validAliasSegment(name) {
		fmt.Fprintf(stderr, "error: invalid password name %q (letters, digits, dot, dash, underscore; no colon)\n", name)
		return 1
	}
	if isReservedKeyringKey(name) {
		fmt.Fprintf(stderr, "error: password name %q is reserved\n", name)
		return 1
	}
	if !validScope(*scope) {
		fmt.Fprintf(stderr, "error: invalid scope %q (read, write, move, admin)\n", *scope)
		return 1
	}
	namespaces, err := parseNamespaceList(*nsFlag)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if s.Locked {
		fmt.Fprintln(stderr, "error: vault is locked; run `aql vault unlock`")
		return 1
	}
	if resolveBackend(s) != BackendFile {
		fmt.Fprintf(stderr, "error: scoped passwords currently require the file backend (this vault uses %s)\n", resolveBackend(s))
		return 1
	}

	// Authenticate as admin with the CURRENT passphrase.
	currentPass, err := readPassphrase(stdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if s.HasPasswordSlots() {
		sess, err := openSession(s, homeDir, currentPass)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		defer sess.Close()
		if err := requireScope(sess, OpAdmin); err != nil {
			fmt.Fprintln(stderr, "error: only an admin password can add passwords")
			return 1
		}
		if existing, _ := s.FindPasswordSlot(name); existing != nil {
			fmt.Fprintf(stderr, "error: password %q already exists\n", name)
			return 1
		}
	} else {
		// Legacy vault: validate the current passphrase by reading the
		// legacy keyring before we migrate.
		if name == "admin" {
			fmt.Fprintln(stderr, "error: the first `password add` reserves the name \"admin\" for your current passphrase; choose another name")
			return 1
		}
		legacy := &fileKeyring{folder: vaultFolder(homeDir), pass: currentPass}
		if _, err := legacy.list(); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}

	newPass, err := readNewPassphrase(stdin, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	migrated := false
	if err := withVaultLock(homeDir, func() error {
		st, err := LoadStore(homeDir)
		if err != nil {
			return err
		}
		if st == nil {
			return errors.New("vault not initialized")
		}
		if !st.HasPasswordSlots() {
			migrated = true
			return migrateLegacyAndAdd(st, homeDir, currentPass, name, *scope, namespaces, newPass)
		}
		return addSlotToEnvelope(st, homeDir, currentPass, name, *scope, namespaces, newPass)
	}); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	reason := "scope=" + *scope + " ns=" + namespacesLabel(namespaces)
	if migrated {
		_ = appendAudit(homeDir, AuditEvent{Action: "vault.password.migrate", Outcome: "ok"})
		fmt.Fprintln(stdout, "migrated vault to envelope format; your current passphrase is now the \"admin\" password")
	}
	_ = appendAudit(homeDir, AuditEvent{Action: "vault.password.add", Alias: name, Outcome: "ok", Reason: reason})
	fmt.Fprintf(stdout, "added password %q (scope=%s, namespaces=%s)\n", name, *scope, namespacesLabel(namespaces))
	return 0
}

// --- rm --------------------------------------------------------------------

func runPasswordRm(args []string, homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault password rm", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rekey := fs.Bool("rekey", false, "rotate the data keys this password could reach (re-encrypts those namespaces)")
	yes := fs.Bool("yes", false, "confirm the removal non-interactively")
	fs.BoolVar(yes, "y", false, "confirm the removal non-interactively (shorthand)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "error: usage: aql vault password rm [--rekey] [--yes] <name>")
		return 1
	}
	name := fs.Arg(0)

	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if !s.HasPasswordSlots() {
		fmt.Fprintln(stderr, "error: vault has no password slots")
		return 1
	}
	sess, err := authenticate(s, homeDir, stdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer sess.Close()
	if err := requireScope(sess, OpAdmin); err != nil {
		fmt.Fprintln(stderr, "error: only an admin password can remove passwords")
		return 1
	}

	if err := confirmDestructive(confirmTier2, name, fmt.Sprintf("remove password %q", name), *yes, true, stdin, stdout); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	if *rekey {
		if err := rekeyRemoveSlot(homeDir, sess, name); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		_ = appendAudit(homeDir, AuditEvent{Action: "vault.password.rm", Alias: name, Outcome: "ok", Reason: "rekey=true"})
		fmt.Fprintf(stdout, "removed password %q and rotated the data keys it could reach\n", name)
		return 0
	}

	if err := mutateStore(homeDir, func(st *Store) error {
		slot, _ := st.FindPasswordSlot(name)
		if slot == nil {
			return fmt.Errorf("password %q not found", name)
		}
		if slot.Scope == ScopeAdmin && !st.OtherFunctionalAdminExists(name) {
			return errors.New("refusing to remove the last admin password")
		}
		st.RemovePasswordSlot(name)
		return nil
	}); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_ = appendAudit(homeDir, AuditEvent{Action: "vault.password.rm", Alias: name, Outcome: "ok", Reason: "rekey=false"})
	fmt.Fprintf(stdout, "removed password %q\n", name)
	fmt.Fprintln(stderr, "note: without --rekey, a holder who kept a copy of an old keyring can still decrypt previously-stored secrets")
	return 0
}

// slotHasNamespaceWrap reports whether a slot holds a wrapped data key
// for namespace ns (under any NDK id).
func slotHasNamespaceWrap(slot *PasswordSlot, ns string) bool {
	for wk := range slot.WrappedKeys {
		if i := strings.LastIndexByte(wk, '#'); i >= 0 && wk[:i] == ns {
			return true
		}
	}
	return false
}

// rekeyRemoveSlot removes a slot AND rotates every namespace data key it
// could reach, so a holder who kept the old passphrase/keyring cannot
// decrypt the re-encrypted secrets. Ordering is commit-new-before-
// destroy-old so a crash never loses data:
//
//	Phase A — mint a new NDK per affected namespace, seal it to every
//	          surviving slot that held that namespace, point CurrentNDK at
//	          it, and commit. Both old and new NDKs are now valid.
//	Phase B — re-encrypt each value in those namespaces under the new NDK
//	          (idempotent: a value already on the new id is skipped).
//	Phase C — drop the old NDK wraps from every slot, remove the slot, and
//	          commit.
//
// A crash before Phase C leaves the old wraps intact, so every value is
// still decryptable; re-running rm --rekey (or vault verify) converges.
func rekeyRemoveSlot(homeDir string, sess *Session, name string) error {
	folder := vaultFolder(homeDir)
	kr := &envelopeFileKeyring{folder: folder}
	ik := sess.IntegrityKey()
	if ik == nil {
		return errors.New("vault: integrity key unavailable")
	}
	return withVaultLock(homeDir, func() error {
		st, err := LoadStore(homeDir)
		if err != nil {
			return err
		}
		removed, _ := st.FindPasswordSlot(name)
		if removed == nil {
			return fmt.Errorf("password %q not found", name)
		}
		if removed.Scope == ScopeAdmin && !st.OtherFunctionalAdminExists(name) {
			return errors.New("refusing to remove the last admin password")
		}
		// Namespaces the removed slot could reach (excluding @integrity).
		nsSet := map[string]bool{}
		for wk := range removed.WrappedKeys {
			if i := strings.LastIndexByte(wk, '#'); i >= 0 {
				if ns := wk[:i]; ns != integrityNamespace {
					nsSet[ns] = true
				}
			}
		}

		// Phase A: mint NDK' per namespace, seal to surviving holders.
		freshNDK := map[string][]byte{}
		freshID := map[string]string{}
		for ns := range nsSet {
			ndk, err := newNDK()
			if err != nil {
				return err
			}
			idb, err := newNDKID()
			if err != nil {
				return err
			}
			idHex := hex.EncodeToString(idb)
			freshNDK[ns] = ndk
			freshID[ns] = idHex
			for i := range st.Passwords {
				slot := &st.Passwords[i]
				if slot.Name == name || !slotHasNamespaceWrap(slot, ns) {
					continue
				}
				want := derivePubMAC(ik, slot.Name, slot.PublicKey, slot.Scope, slot.Namespaces)
				if !hmac.Equal([]byte(want), []byte(slot.PubMAC)) {
					return fmt.Errorf("slot %q failed its public-key integrity check; aborting rekey", slot.Name)
				}
				pubBytes, err := base64.StdEncoding.DecodeString(slot.PublicKey)
				if err != nil || len(pubBytes) != 32 {
					return fmt.Errorf("slot %q has an invalid public key", slot.Name)
				}
				var pub [32]byte
				copy(pub[:], pubBytes)
				sealed, err := sealNDK(ndk, &pub)
				if err != nil {
					return err
				}
				slot.WrappedKeys[ns+"#"+idHex] = sealed
			}
			st.CurrentNDK[ns] = idHex
		}
		if err := SaveStore(homeDir, st); err != nil {
			return err
		}

		// Phase B: re-encrypt every value in the affected namespaces.
		for _, a := range st.Aliases {
			ns := valueNamespace(a.Name)
			if !nsSet[ns] {
				continue
			}
			sealedOld, err := kr.Get(a.Name)
			if err == ErrNotFound {
				continue
			}
			if err != nil {
				return err
			}
			plain, err := openValue(sealedOld, ns, a.Name, sess.ndkByID)
			if err != nil {
				return fmt.Errorf("re-encrypting %s: %w", a.Name, err)
			}
			idb, err := hex.DecodeString(freshID[ns])
			if err != nil {
				return err
			}
			resealed, err := sealValue(freshNDK[ns], idb, ns, a.Name, plain)
			if err != nil {
				return err
			}
			if err := kr.Set(a.Name, resealed); err != nil {
				return err
			}
		}

		// Phase C: drop the old NDK wraps and remove the slot.
		for ns := range nsSet {
			keep := ns + "#" + freshID[ns]
			for i := range st.Passwords {
				for wk := range st.Passwords[i].WrappedKeys {
					if j := strings.LastIndexByte(wk, '#'); j >= 0 && wk[:j] == ns && wk != keep {
						delete(st.Passwords[i].WrappedKeys, wk)
					}
				}
			}
		}
		st.RemovePasswordSlot(name)
		return SaveStore(homeDir, st)
	})
}

// --- helpers ---------------------------------------------------------------

// readNewPassphrase reads and confirms the NEW slot password.
func readNewPassphrase(stdin io.Reader, stdout io.Writer) (string, error) {
	if p := envNewPassphrase(); p != "" {
		return p, nil
	}
	if stdin == nil {
		return "", errors.New("no terminal to prompt for the new password; set " + EnvNewPassphrase)
	}
	ir := auth.NewInputReader(stdin)
	p1, err := ir.ReadPassword("Set password: ", stdout)
	if err != nil {
		return "", err
	}
	p2, err := ir.ReadPassword("Confirm password: ", stdout)
	if err != nil {
		return "", err
	}
	if p1 != p2 {
		return "", errors.New("passwords did not match")
	}
	if p1 == "" {
		return "", errors.New("empty password")
	}
	return p1, nil
}

func envNewPassphrase() string { return os.Getenv(EnvNewPassphrase) }

// parseNamespaceList parses a --namespaces value into stored-form names
// (""=root), or ["*"] for all.
func parseNamespaceList(v string) ([]string, error) {
	if strings.TrimSpace(v) == "" || v == "*" {
		return []string{"*"}, nil
	}
	var out []string
	seen := map[string]bool{}
	for _, raw := range strings.Split(v, ",") {
		tok := strings.TrimSpace(raw)
		switch {
		case tok == "*":
			return []string{"*"}, nil
		case tok == rootNamespaceRef || tok == "":
			tok = "" // root
		case !validNamespaceName(tok):
			return nil, fmt.Errorf("invalid namespace %q", tok)
		}
		if !seen[tok] {
			seen[tok] = true
			out = append(out, tok)
		}
	}
	if len(out) == 0 {
		return []string{"*"}, nil
	}
	return out, nil
}

func containsStar(ns []string) bool {
	for _, n := range ns {
		if n == "*" {
			return true
		}
	}
	return false
}

// buildSlot constructs a PasswordSlot for pass, sealing the supplied
// NDKs (keyed by namespace) to a fresh keypair. currentNDK supplies the
// id for each namespace's WrappedKeys entry; integrityKey signs PubMAC.
func buildSlot(name, scope string, namespaces []string, pass string, vaultSalt []byte, grant map[string][]byte, currentNDK map[string]string, integrityKey []byte) (PasswordSlot, error) {
	var zero PasswordSlot
	master, err := vaultMasterKEK(pass, vaultSalt)
	if err != nil {
		return zero, err
	}
	defer zeroize(master)
	salt, err := newRandom(keyringSaltLen)
	if err != nil {
		return zero, err
	}
	kek, err := slotKEK(master, salt)
	if err != nil {
		return zero, err
	}
	defer zeroize(kek)
	pub, priv, err := newSlotKeypair()
	if err != nil {
		return zero, err
	}
	enc, err := sealPrivKey(kek, priv[:], privAAD(name, scope))
	if err != nil {
		return zero, err
	}
	pubB64 := base64.StdEncoding.EncodeToString(pub[:])
	wrapped := map[string]string{}
	for ns, ndk := range grant {
		id := currentNDK[ns]
		if id == "" {
			return zero, fmt.Errorf("internal: no current id for namespace %q", nsLabel(ns))
		}
		sealed, err := sealNDK(ndk, pub)
		if err != nil {
			return zero, err
		}
		wrapped[ns+"#"+id] = sealed
	}
	return PasswordSlot{
		Name:        name,
		Scope:       scope,
		Namespaces:  namespaces,
		Salt:        base64.StdEncoding.EncodeToString(salt),
		Verifier:    deriveVerifier(kek, name, scope),
		PublicKey:   pubB64,
		PubMAC:      derivePubMAC(integrityKey, name, pubB64, scope, namespaces),
		EncPrivKey:  enc,
		WrappedKeys: wrapped,
	}, nil
}

// grantNDKsFor selects the NDKs a slot of the given scope/namespaces
// should receive from the full set, always including the integrity key.
func grantNDKsFor(all map[string][]byte, scope string, namespaces []string) map[string][]byte {
	out := map[string][]byte{integrityNamespace: all[integrityNamespace]}
	if scope == ScopeAdmin || containsStar(namespaces) {
		for ns, k := range all {
			out[ns] = k
		}
		return out
	}
	for _, ns := range namespaces {
		if k, ok := all[ns]; ok {
			out[ns] = k
		}
	}
	return out
}

// migrateLegacyAndAdd converts a legacy single-passphrase vault to the
// envelope format and adds the requested slot, committing the store
// (with the recoverable admin slot) before re-sealing the keyring.
func migrateLegacyAndAdd(s *Store, homeDir, currentPass, newName, newScope string, newNS []string, newPass string) error {
	folder := vaultFolder(homeDir)
	legacy := &fileKeyring{folder: folder, pass: currentPass}

	// 1. Read every existing secret with the current passphrase.
	type entry struct{ alias, ns, value string }
	var entries []entry
	for _, a := range s.Aliases {
		v, err := legacy.Get(a.Name)
		if err == ErrNotFound {
			continue
		}
		if err != nil {
			return fmt.Errorf("reading secret %q (wrong passphrase?): %w", a.Name, err)
		}
		entries = append(entries, entry{a.Name, valueNamespace(a.Name), v})
	}

	// 2. Generate the shared vault salt and per-namespace data keys.
	vsalt, err := newRandom(keyringSaltLen)
	if err != nil {
		return err
	}
	s.VaultSalt = base64.StdEncoding.EncodeToString(vsalt)
	s.CurrentNDK = map[string]string{}
	ndks := map[string][]byte{}
	mkNDK := func(ns string) error {
		if _, ok := ndks[ns]; ok {
			return nil
		}
		ndk, err := newNDK()
		if err != nil {
			return err
		}
		id, err := newNDKID()
		if err != nil {
			return err
		}
		ndks[ns] = ndk
		s.CurrentNDK[ns] = hex.EncodeToString(id)
		return nil
	}
	if err := mkNDK(integrityNamespace); err != nil {
		return err
	}
	if err := mkNDK(""); err != nil { // root always exists
		return err
	}
	for _, e := range entries {
		if err := mkNDK(e.ns); err != nil {
			return err
		}
	}
	if !containsStar(newNS) {
		for _, ns := range newNS {
			if err := mkNDK(ns); err != nil {
				return err
			}
		}
	}
	integrityKey := ndks[integrityNamespace]

	// 3. Seed admin slot (from the current passphrase) + the requested slot.
	adminSlot, err := buildSlot("admin", ScopeAdmin, []string{"*"}, currentPass, vsalt, ndks, s.CurrentNDK, integrityKey)
	if err != nil {
		return err
	}
	reqSlot, err := buildSlot(newName, newScope, newNS, newPass, vsalt, grantNDKsFor(ndks, newScope, newNS), s.CurrentNDK, integrityKey)
	if err != nil {
		return err
	}
	s.Passwords = []PasswordSlot{adminSlot, reqSlot}

	// 4. Commit the store FIRST so the admin slot (the only durable copy
	//    of every NDK) lands before the legacy keyring is overwritten.
	if err := SaveStore(homeDir, s); err != nil {
		return err
	}

	// 5. Re-seal every secret into the envelope keyring (overwrites the
	//    legacy format-1 file at the same path).
	env := &envelopeFileKeyring{folder: folder}
	ekf := &envelopeKeyringFile{Entries: map[string]string{}}
	for _, e := range entries {
		idBytes, err := hex.DecodeString(s.CurrentNDK[e.ns])
		if err != nil {
			return err
		}
		sealed, err := sealValue(ndks[e.ns], idBytes, e.ns, e.alias, e.value)
		if err != nil {
			return err
		}
		ekf.Entries[e.alias] = sealed
	}
	return env.saveFile(ekf)
}

// addSlotToEnvelope adds a slot to an already-envelope vault. The admin
// authenticates, unseals the relevant NDKs, and re-seals them to the new
// slot's public key. Granting a namespace that has no NDK yet generates
// one and seals it to every admin slot too.
func addSlotToEnvelope(s *Store, homeDir, currentPass, newName, newScope string, newNS []string, newPass string) error {
	sess, err := openSession(s, homeDir, currentPass)
	if err != nil {
		return err
	}
	defer sess.Close()
	if err := requireScope(sess, OpAdmin); err != nil {
		return errors.New("only an admin password can add passwords")
	}
	if existing, _ := s.FindPasswordSlot(newName); existing != nil {
		return fmt.Errorf("password %q already exists", newName)
	}
	vsalt, err := base64.StdEncoding.DecodeString(s.VaultSalt)
	if err != nil {
		return errors.New("vault: missing vault salt")
	}
	integrityKey := sess.IntegrityKey()
	if integrityKey == nil {
		return errors.New("vault: integrity key unavailable")
	}

	// Resolve the NDKs to grant from the admin session, generating any
	// brand-new namespace's NDK and sealing it to every admin slot.
	grant := map[string][]byte{integrityNamespace: integrityKey}
	wantNS := newNS
	if newScope == ScopeAdmin || containsStar(newNS) {
		wantNS = nil
		for ns := range s.CurrentNDK {
			if ns == integrityNamespace {
				continue
			}
			wantNS = append(wantNS, ns)
		}
	}
	for _, ns := range wantNS {
		id := s.CurrentNDK[ns]
		if id != "" {
			if k, ok := sess.ndks[ns+"#"+id]; ok {
				grant[ns] = k
				continue
			}
			return fmt.Errorf("admin session lacks the data key for namespace %q", nsLabel(ns))
		}
		// Brand-new namespace: generate an NDK and seal it to admin slots.
		ndk, err := newNDK()
		if err != nil {
			return err
		}
		idb, err := newNDKID()
		if err != nil {
			return err
		}
		idHex := hex.EncodeToString(idb)
		if s.CurrentNDK == nil {
			s.CurrentNDK = map[string]string{}
		}
		s.CurrentNDK[ns] = idHex
		grant[ns] = ndk
		if err := sealNDKToAdmins(s, ns, idHex, ndk, integrityKey); err != nil {
			return err
		}
	}

	slot, err := buildSlot(newName, newScope, newNS, newPass, vsalt, grant, s.CurrentNDK, integrityKey)
	if err != nil {
		return err
	}
	s.UpsertPasswordSlot(slot)
	return SaveStore(homeDir, s)
}

// sealWrapped seals each granted NDK to pub, keyed by "ns#currentId".
func sealWrapped(grant map[string][]byte, currentNDK map[string]string, pub *[32]byte) (map[string]string, error) {
	wrapped := map[string]string{}
	for ns, ndk := range grant {
		id := currentNDK[ns]
		if id == "" {
			return nil, fmt.Errorf("internal: no current id for namespace %q", nsLabel(ns))
		}
		sealed, err := sealNDK(ndk, pub)
		if err != nil {
			return nil, err
		}
		wrapped[ns+"#"+id] = sealed
	}
	return wrapped, nil
}

// resolveGrant returns the data keys to seal for a namespace list,
// pulling existing NDKs from the admin session and generating (and
// admin-sealing) any brand-new namespace. Always includes the integrity
// key. Mutates st.CurrentNDK / admin WrappedKeys when generating.
func resolveGrant(st *Store, sess *Session, namespaces []string) (map[string][]byte, error) {
	ik := sess.IntegrityKey()
	if ik == nil {
		return nil, errors.New("vault: integrity key unavailable")
	}
	grant := map[string][]byte{integrityNamespace: ik}
	var wantNS []string
	if containsStar(namespaces) {
		for ns := range st.CurrentNDK {
			if ns != integrityNamespace {
				wantNS = append(wantNS, ns)
			}
		}
	} else {
		wantNS = namespaces
	}
	for _, ns := range wantNS {
		if id := st.CurrentNDK[ns]; id != "" {
			k, ok := sess.ndks[ns+"#"+id]
			if !ok {
				return nil, fmt.Errorf("admin session lacks the data key for namespace %q", nsLabel(ns))
			}
			grant[ns] = k
			continue
		}
		ndk, err := newNDK()
		if err != nil {
			return nil, err
		}
		idb, err := newNDKID()
		if err != nil {
			return nil, err
		}
		idHex := hex.EncodeToString(idb)
		if st.CurrentNDK == nil {
			st.CurrentNDK = map[string]string{}
		}
		st.CurrentNDK[ns] = idHex
		grant[ns] = ndk
		if err := sealNDKToAdmins(st, ns, idHex, ndk, ik); err != nil {
			return nil, err
		}
	}
	return grant, nil
}

// runPasswordAssign changes an existing slot's namespace allow-list. It
// is admin-driven and needs no target passphrase: the admin re-wraps the
// namespace data keys to the slot's authenticated public key. Scope
// changes are not supported here (scope is bound to the holder's
// passphrase) — use rm + add to re-scope.
func runPasswordAssign(args []string, homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault password assign", flag.ContinueOnError)
	fs.SetOutput(stderr)
	nsFlag := fs.String("namespaces", "", "new namespace allow-list ('*' = all, ':' = root)")
	scopeFlag := fs.String("scope", "", "(unsupported here — scope changes require rm + add)")
	yes := fs.Bool("yes", false, "confirm non-interactively")
	fs.BoolVar(yes, "y", false, "confirm non-interactively (shorthand)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "error: usage: aql vault password assign --namespaces=... <name>")
		return 1
	}
	if *scopeFlag != "" {
		fmt.Fprintln(stderr, "error: changing scope via assign is not supported (scope is bound to the holder's passphrase); use `vault password rm` then `add` to re-scope")
		return 1
	}
	if *nsFlag == "" {
		fmt.Fprintln(stderr, "error: assign requires --namespaces")
		return 1
	}
	name := fs.Arg(0)
	namespaces, err := parseNamespaceList(*nsFlag)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if s.Locked {
		fmt.Fprintln(stderr, "error: vault is locked; run `aql vault unlock`")
		return 1
	}
	target, _ := s.FindPasswordSlot(name)
	if target == nil {
		fmt.Fprintf(stderr, "error: no password named %q\n", name)
		return 1
	}
	if target.Scope == ScopeAdmin {
		fmt.Fprintln(stderr, "error: admin passwords always cover every namespace; reassigning is a no-op")
		return 1
	}

	sess, err := authenticate(s, homeDir, stdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer sess.Close()
	if err := requireScope(sess, OpAdmin); err != nil {
		fmt.Fprintln(stderr, "error: only an admin password can reassign namespaces")
		return 1
	}
	if err := confirmDestructive(confirmTier1, "", fmt.Sprintf("reassign namespaces for password %q", name), *yes, false, stdin, stdout); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	if err := withVaultLock(homeDir, func() error {
		st, err := LoadStore(homeDir)
		if err != nil {
			return err
		}
		slot, idx := st.FindPasswordSlot(name)
		if slot == nil {
			return fmt.Errorf("password %q not found", name)
		}
		// M1: authenticate the slot's public key before re-sealing to it.
		want := derivePubMAC(sess.IntegrityKey(), slot.Name, slot.PublicKey, slot.Scope, slot.Namespaces)
		if !hmac.Equal([]byte(want), []byte(slot.PubMAC)) {
			return errors.New("slot public key failed its integrity check (tampered?); refusing to re-seal")
		}
		pubBytes, err := base64.StdEncoding.DecodeString(slot.PublicKey)
		if err != nil || len(pubBytes) != 32 {
			return errors.New("invalid slot public key")
		}
		var pub [32]byte
		copy(pub[:], pubBytes)
		grant, err := resolveGrant(st, sess, namespaces)
		if err != nil {
			return err
		}
		wrapped, err := sealWrapped(grant, st.CurrentNDK, &pub)
		if err != nil {
			return err
		}
		st.Passwords[idx].WrappedKeys = wrapped
		st.Passwords[idx].Namespaces = namespaces
		st.Passwords[idx].PubMAC = derivePubMAC(sess.IntegrityKey(), slot.Name, slot.PublicKey, slot.Scope, namespaces)
		st.Passwords[idx].UpdatedAt = nowRFC3339Nano()
		return SaveStore(homeDir, st)
	}); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_ = appendAudit(homeDir, AuditEvent{Action: "vault.password.assign", Alias: name, Outcome: "ok", Reason: "ns=" + namespacesLabel(namespaces)})
	fmt.Fprintf(stdout, "reassigned %q to namespaces %s\n", name, namespacesLabel(namespaces))
	return 0
}

// runPasswordSet rotates a slot's own passphrase: the holder supplies
// the current and new password; the slot's keypair (and thus its NDK
// wraps and public key) is unchanged. Admin reset of a forgotten
// password is rm + add.
func runPasswordSet(args []string, homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault password set", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "error: usage: aql vault password set <name>")
		return 1
	}
	name := fs.Arg(0)
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if !s.HasPasswordSlots() {
		fmt.Fprintln(stderr, "error: vault has no password slots")
		return 1
	}
	currentPass, err := readPassphrase(stdin, stdout, "Current password: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	sess, err := openSession(s, homeDir, currentPass)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer sess.Close()
	if sess.Slot.Name != name {
		fmt.Fprintf(stderr, "error: that passphrase is not password %q (set rotates a slot's own password; admin reset is rm + add)\n", name)
		return 1
	}
	newPass, err := readNewPassphrase(stdin, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	vsalt, err := base64.StdEncoding.DecodeString(s.VaultSalt)
	if err != nil {
		fmt.Fprintf(stderr, "error: vault: missing vault salt\n")
		return 1
	}
	if err := withVaultLock(homeDir, func() error {
		st, err := LoadStore(homeDir)
		if err != nil {
			return err
		}
		slot, idx := st.FindPasswordSlot(name)
		if slot == nil {
			return fmt.Errorf("password %q not found", name)
		}
		master, err := vaultMasterKEK(newPass, vsalt)
		if err != nil {
			return err
		}
		defer zeroize(master)
		newSalt, err := newRandom(keyringSaltLen)
		if err != nil {
			return err
		}
		kek, err := slotKEK(master, newSalt)
		if err != nil {
			return err
		}
		defer zeroize(kek)
		enc, err := sealPrivKey(kek, sess.privKey, privAAD(slot.Name, slot.Scope))
		if err != nil {
			return err
		}
		st.Passwords[idx].Salt = base64.StdEncoding.EncodeToString(newSalt)
		st.Passwords[idx].Verifier = deriveVerifier(kek, slot.Name, slot.Scope)
		st.Passwords[idx].EncPrivKey = enc
		st.Passwords[idx].UpdatedAt = nowRFC3339Nano()
		return SaveStore(homeDir, st)
	}); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_ = appendAudit(homeDir, AuditEvent{Action: "vault.password.set", Alias: name, Outcome: "ok"})
	fmt.Fprintf(stdout, "rotated the password for %q\n", name)
	return 0
}

// sealNDKToAdmins seals a freshly generated NDK into every admin slot's
// WrappedKeys so admins retain full coverage. Each admin slot's public
// key is authenticated against its PubMAC (keyed by integrityKey) before
// the NDK is sealed to it, so a tampered admin public key cannot harvest
// a newly minted data key.
func sealNDKToAdmins(s *Store, ns, idHex string, ndk, integrityKey []byte) error {
	for i := range s.Passwords {
		slot := &s.Passwords[i]
		if slot.Scope != ScopeAdmin {
			continue
		}
		want := derivePubMAC(integrityKey, slot.Name, slot.PublicKey, slot.Scope, slot.Namespaces)
		if !hmac.Equal([]byte(want), []byte(slot.PubMAC)) {
			return fmt.Errorf("admin slot %q failed its public-key integrity check; refusing to seal", slot.Name)
		}
		pubBytes, err := base64.StdEncoding.DecodeString(slot.PublicKey)
		if err != nil || len(pubBytes) != 32 {
			return fmt.Errorf("admin slot %q has an invalid public key", slot.Name)
		}
		var pub [32]byte
		copy(pub[:], pubBytes)
		sealed, err := sealNDK(ndk, &pub)
		if err != nil {
			return err
		}
		if slot.WrappedKeys == nil {
			slot.WrappedKeys = map[string]string{}
		}
		slot.WrappedKeys[ns+"#"+idHex] = sealed
	}
	return nil
}
