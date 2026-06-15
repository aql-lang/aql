package vault

// api.go — a small exported surface so other commands (e.g. `aql login`
// / `aql publish`) can read and write vault secrets without importing
// the vault's internals. Both helpers go through the normal
// authenticate/Session path, so they work on legacy single-passphrase
// vaults and on envelope (scoped-password) vaults alike, sourcing the
// passphrase from AQL_VAULT_PASSPHRASE or an interactive prompt.

import "io"

// ReadSecret returns the secret stored under alias. The vault must be
// initialized; alias must already exist. ns/scope rules apply (read
// scope, namespace coverage).
func ReadSecret(homeDir, alias string, stdin io.Reader, stdout io.Writer) (string, error) {
	s, err := requireStore(homeDir)
	if err != nil {
		return "", err
	}
	if s.Locked {
		return "", errLocked
	}
	_, name, err := findAliasRef(s, alias)
	if err != nil {
		return "", err
	}
	sess, err := authenticate(s, homeDir, stdin, stdout, "Vault passphrase: ")
	if err != nil {
		return "", err
	}
	defer sess.Close()
	ns, _ := splitAlias(name)
	if err := requireScopeNS(sess, OpRead, ns); err != nil {
		return "", err
	}
	return sess.getValue(name, ns)
}

// WriteSecret stores value under alias (creating or replacing it),
// tagging the metadata with source. The vault must be initialized.
func WriteSecret(homeDir, alias, value, source string, stdin io.Reader, stdout io.Writer) error {
	s, err := requireStore(homeDir)
	if err != nil {
		return err
	}
	if s.Locked {
		return errLocked
	}
	name, err := resolveAliasRef(s, alias)
	if err != nil {
		return err
	}
	ns, _ := splitAlias(name)
	sess, err := authenticate(s, homeDir, stdin, stdout, "Vault passphrase: ")
	if err != nil {
		return err
	}
	defer sess.Close()
	if err := requireScopeNS(sess, OpWrite, ns); err != nil {
		return err
	}
	if err := writeSecret(homeDir, sess, name, ns, value); err != nil {
		return err
	}
	if err := mutateStore(homeDir, func(st *Store) error {
		st.UpsertAlias(Alias{Name: name, Namespace: ns, Source: source})
		return nil
	}); err != nil {
		return err
	}
	sealIntegrity(homeDir, sess)
	_ = appendAudit(homeDir, AuditEvent{Action: "vault.add", Alias: name, Outcome: "ok", Reason: "source=" + source})
	return nil
}
