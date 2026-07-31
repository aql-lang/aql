package vault

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

// runMv implements `boru vault mv <src> <dst>` (alias: rename). It
// renames one key or moves a whole namespace:
//
//	boru vault mv key proj:key        # move a key into a namespace
//	boru vault mv proj:key other:k2   # move and rename across namespaces
//	boru vault mv proj: other:        # rename namespace proj to other
//	boru vault mv proj: :             # move every proj key to root
//
// Alias references follow the usual resolution rule (bare names use
// the default namespace, :name forces root); a trailing colon (`ns:`,
// or `:` alone for root) denotes a whole namespace, and both sides
// must be the same kind. Namespace selection matches the reporting
// filters: a legacy alias whose namespace is only a metadata tag is
// selected too, and moving it re-tags or properly qualifies it.
//
// The keyring sequence is copy → verify → rename records → delete
// old, with every destination pre-flighted first, so a failure mid-
// batch can leave a duplicate but never lose a secret, and a bulk
// rename never half-commits the store. Capabilities re-bind to the
// new name by default (the secret is unchanged; proxy clients must
// switch to the new alias path) or are revoked with --revoke-caps.
func runMv(args []string, homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault mv", flag.ContinueOnError)
	fs.SetOutput(stderr)
	revokeCaps := fs.Bool("revoke-caps", false, "revoke capabilities bound to moved aliases instead of re-binding them")
	dryRun := fs.Bool("dry-run", false, "show what would move without changing anything")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() != 2 {
		fmt.Fprintf(stderr, "error: usage: boru vault mv [--revoke-caps] [--dry-run] <src> <dst>  (aliases like [ns:]name | :name, or whole namespaces like ns: | :)\n")
		return 1
	}
	srcRef, dstRef := fs.Arg(0), fs.Arg(1)

	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if s.Locked {
		fmt.Fprintln(stderr, "error: vault is locked; run `boru vault unlock`")
		return 1
	}

	srcNS, srcIsNS, err := parseNamespaceRef(srcRef)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	dstNS, dstIsNS, err := parseNamespaceRef(dstRef)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if srcIsNS != dstIsNS {
		fmt.Fprintln(stderr, "error: source and destination must both be aliases, or both namespaces (ns: / :)")
		return 1
	}

	type move struct{ from, to string }
	var moves []move
	if srcIsNS {
		if srcNS == dstNS {
			fmt.Fprintf(stderr, "error: source and destination namespace are the same\n")
			return 1
		}
		for _, a := range s.SortedAliases() {
			if aliasNamespace(a) != srcNS {
				continue
			}
			moves = append(moves, move{from: a.Name, to: requalify(a.Name, dstNS)})
		}
		if len(moves) == 0 {
			fmt.Fprintf(stderr, "error: no aliases in namespace %q\n", nsLabel(srcNS))
			return 1
		}
	} else {
		_, from, err := findAliasRef(s, srcRef)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		to, err := resolveAliasRef(s, dstRef)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		if from == to {
			fmt.Fprintf(stderr, "error: source and destination are the same alias %q\n", from)
			return 1
		}
		moves = append(moves, move{from: from, to: to})
	}

	// Pre-flight every destination before touching anything: it must
	// not exist already and must be unique within the batch. A move
	// whose name is unchanged (a legacy metadata-tagged alias going to
	// root) is a pure re-tag and skips these checks and the keyring.
	taken := map[string]bool{}
	for _, m := range moves {
		if m.from == m.to {
			continue
		}
		if a, _ := s.FindAlias(m.to); a != nil {
			fmt.Fprintf(stderr, "error: destination alias %q already exists\n", m.to)
			return 1
		}
		if taken[m.to] {
			fmt.Fprintf(stderr, "error: two source aliases would both become %q\n", m.to)
			return 1
		}
		taken[m.to] = true
	}

	if *dryRun {
		for _, m := range moves {
			if m.from == m.to {
				fmt.Fprintf(stdout, "would re-tag %s (namespace %s -> %s)\n", m.from, nsLabel(srcNS), nsLabel(dstNS))
				continue
			}
			fmt.Fprintf(stdout, "would move %s -> %s\n", m.from, m.to)
		}
		return 0
	}

	sess, err := authenticate(s, homeDir, stdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	defer sess.Close()
	// A move needs move scope over both the source and destination
	// namespaces; on an envelope vault it also needs the data keys for
	// both (decrypt under source, re-seal under destination).
	for _, m := range moves {
		if err := requireScopeNS(sess, OpMove, valueNamespace(m.from)); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		if err := requireScopeNS(sess, OpMove, valueNamespace(m.to)); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}

	// Phase 1: copy every secret to its new name and verify the copy.
	// On any failure, remove the copies already made so a partial
	// batch never commits; the originals are untouched.
	var copied []string
	abort := func(from string, err error) int {
		for _, c := range copied {
			_ = c7sessDeleteValue(sess, c)
		}
		fmt.Fprintf(stderr, "error: moving %s: %s\n", from, err)
		return 1
	}
	for _, m := range moves {
		if m.from == m.to {
			continue
		}
		// Decrypt under the source namespace, re-seal under the
		// destination (the sealed value's AAD binds the alias+namespace,
		// so a raw copy would not open under the new name). In legacy mode
		// these are raw passthrough.
		v, err := c7sessGetValue(sess, m.from, valueNamespace(m.from))
		if err != nil {
			return abort(m.from, err)
		}
		if err := writeSecret(homeDir, sess, m.to, valueNamespace(m.to), v); err != nil {
			return abort(m.from, err)
		}
		got, err := c7sessGetValue(sess, m.to, valueNamespace(m.to))
		if err != nil {
			return abort(m.from, err)
		}
		if got != v {
			_ = c7sessDeleteValue(sess, m.to)
			return abort(m.from, fmt.Errorf("copy to %q did not verify", m.to))
		}
		copied = append(copied, m.to)
	}

	// Phase 2: rename the records and persist, under the vault lock and
	// against a freshly-loaded store, re-checking destination
	// collisions so a concurrent writer cannot be clobbered. On any
	// failure the keyring copies are rolled back, returning to the
	// pre-move state (the store on disk still references the old names).
	touched := 0
	if err := mutateStore(homeDir, func(s *Store) error {
		for _, m := range moves {
			if m.from == m.to {
				continue
			}
			if a, _ := s.FindAlias(m.to); a != nil {
				return fmt.Errorf("destination alias %q now exists; aborting", m.to)
			}
		}
		touched = 0
		for _, m := range moves {
			n, _ := s.RenameAlias(m.from, m.to, *revokeCaps)
			touched += n
		}
		return nil
	}); err != nil {
		for _, c := range copied {
			_ = c7sessDeleteValue(sess, c)
		}
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	// Phase 3: delete the old keyring entries. The store already
	// references the new names, so a failure here leaves only an
	// orphaned (unreferenced) copy — warn rather than fail.
	for _, m := range moves {
		if m.from == m.to {
			continue
		}
		if err := c7sessDeleteValue(sess, m.from); err != nil {
			fmt.Fprintf(stderr, "warning: could not remove old keyring entry %q: %s\n", m.from, err)
		}
	}
	sealIntegrity(homeDir, sess)

	for _, m := range moves {
		_ = appendAudit(homeDir, AuditEvent{
			Action: "vault.mv", Alias: m.from, Outcome: "ok", Reason: "to=" + m.to,
		})
		if m.from == m.to {
			fmt.Fprintf(stdout, "re-tagged %s (namespace %s -> %s)\n", m.from, nsLabel(srcNS), nsLabel(dstNS))
			continue
		}
		fmt.Fprintf(stdout, "moved %s -> %s\n", m.from, m.to)
	}
	if touched > 0 {
		if *revokeCaps {
			fmt.Fprintf(stdout, "revoked %d capability(s)\n", touched)
		} else {
			fmt.Fprintf(stdout, "re-bound %d capability(s); proxy clients must use the new alias path\n", touched)
		}
	}
	return 0
}

// parseNamespaceRef reports whether ref denotes a whole namespace —
// `ns:` (trailing colon) or `:` alone (root) — returning its name
// ("" for root). Refs without a trailing colon are not namespace
// references (isNS false, no error); a trailing colon with an invalid
// namespace part is an error.
func parseNamespaceRef(ref string) (ns string, isNS bool, err error) {
	if ref == rootNamespaceRef {
		return "", true, nil
	}
	if !strings.HasSuffix(ref, ":") {
		return "", false, nil
	}
	ns = strings.TrimSuffix(ref, ":")
	if !validNamespaceName(ns) {
		return "", true, fmt.Errorf("invalid namespace reference %q", ref)
	}
	return ns, true, nil
}

// nsLabel renders a namespace name for messages, with root spelled
// out instead of an empty string.
func nsLabel(ns string) string {
	if ns == "" {
		return "(root)"
	}
	return ns
}
