package vault

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/term"

	"github.com/aql-lang/aql/cmd/go/internal/auth"
)

// Export bundles let a vault be moved between machines and backends:
// they carry alias metadata and the secret values, re-encrypted under a
// passphrase you choose, independent of the host keystore. A `file`
// vault is already portable as two files; an export bundle is how you
// move secrets *out of* an OS-keychain or 1Password vault to a
// different OS.
//
// On-disk envelope (self-describing, per ADR-004):
//
//	"AQLX" | format(1 byte) | salt(16) | nonce(12) | ciphertext|tag
//
// The header bytes + salt are bound in as AEAD additional data, so the
// format byte cannot be downgraded by tampering. The plaintext is the
// JSON exportBundle below, which carries its own schema version.
const (
	// EnvExportPassphrase supplies the bundle passphrase non-interactively
	// for both export and import.
	EnvExportPassphrase = "AQL_VAULT_EXPORT_PASSPHRASE"

	exportMagic = "AQLX"
	// exportEnvelopeFormat versions the crypto envelope (KDF/cipher/layout).
	exportEnvelopeFormat = 1
	// exportVersion versions the inner JSON schema.
	exportVersion = 1
)

// exportBundle is the decrypted payload of an export.
type exportBundle struct {
	Version    int           `json:"version"`
	ExportedAt string        `json:"exported_at"`
	Aliases    []exportAlias `json:"aliases"`
}

// exportAlias is one secret plus the metadata worth carrying with it.
type exportAlias struct {
	Name      string `json:"name"`
	Provider  string `json:"provider,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Source    string `json:"source,omitempty"`
	Value     string `json:"value"`
}

// isExportBundle reports whether data is an encrypted export bundle, by
// its magic header. Used to dispatch `vault import` between bundles and
// .env files.
func isExportBundle(data []byte) bool {
	return len(data) >= len(exportMagic) && string(data[:len(exportMagic)]) == exportMagic
}

// --- export ----------------------------------------------------------------

func runExport(args []string, homeDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "", "write the encrypted bundle to this file (default: stdout)")
	namespace := fs.String("namespace", "", "only export aliases in this namespace (':' = root only)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	nsFilter, nsFiltered, err := normalizeNSFilter(*namespace)
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

	selected, err := selectExportAliases(s, fs.Args(), nsFilter, nsFiltered)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if len(selected) == 0 {
		fmt.Fprintln(stderr, "error: no matching aliases to export")
		return 1
	}

	// Never spew a binary bundle onto an interactive terminal.
	if *out == "" && isTerminalWriter(stdout) {
		fmt.Fprintln(stderr, "error: refusing to write a binary bundle to the terminal; use --out=FILE or redirect stdout")
		return 1
	}

	kr, err := openKeyring(s, homeDir, stdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	bundle := exportBundle{Version: exportVersion, ExportedAt: time.Now().UTC().Format(time.RFC3339)}
	for _, a := range selected {
		v, err := kr.Get(a.Name)
		if err != nil {
			fmt.Fprintf(stderr, "error: reading %s: %s\n", a.Name, err)
			return 1
		}
		bundle.Aliases = append(bundle.Aliases, exportAlias{
			Name: a.Name, Provider: a.Provider, Namespace: a.Namespace, Source: a.Source, Value: v,
		})
	}

	pass, err := exportSealPassphrase(stdin, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if pass == "" {
		fmt.Fprintln(stderr, "error: export passphrase must not be empty; the bundle holds your secrets in re-encrypted form")
		return 1
	}

	plain, err := json.Marshal(bundle)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	blob, err := sealExport(plain, pass)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	if *out != "" {
		if err := os.WriteFile(*out, blob, 0600); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		fmt.Fprintf(stderr, "exported %d secret(s) to %s\n", len(bundle.Aliases), *out)
	} else {
		if _, err := stdout.Write(blob); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		fmt.Fprintf(stderr, "exported %d secret(s)\n", len(bundle.Aliases))
	}
	_ = appendAudit(homeDir, AuditEvent{
		Action: "vault.export", Outcome: "ok",
		Reason: fmt.Sprintf("aliases=%d", len(bundle.Aliases)),
	})
	return 0
}

// selectExportAliases returns the aliases to export: the referenced
// ones if any are given (each resolved through the namespace rule, so
// bare names follow the active default), otherwise all, filtered by
// namespace when one was specified ("" with filtered=true means root
// only).
func selectExportAliases(s *Store, names []string, nsFilter string, nsFiltered bool) ([]Alias, error) {
	var out []Alias
	if len(names) > 0 {
		for _, ref := range names {
			a, _, err := findAliasRef(s, ref)
			if err != nil {
				return nil, err
			}
			if !nsFiltered || aliasNamespace(*a) == nsFilter {
				out = append(out, *a)
			}
		}
		return out, nil
	}
	for _, a := range s.SortedAliases() {
		if !nsFiltered || aliasNamespace(a) == nsFilter {
			out = append(out, a)
		}
	}
	return out, nil
}

// exportSealPassphrase reads the bundle passphrase from the environment
// or, failing that, prompts for it twice (with confirmation).
func exportSealPassphrase(stdin io.Reader, stdout io.Writer) (string, error) {
	if p := os.Getenv(EnvExportPassphrase); p != "" {
		return p, nil
	}
	ir := auth.NewInputReader(stdin)
	p1, err := ir.ReadPassword("Set export passphrase: ", stdout)
	if err != nil {
		return "", err
	}
	p2, err := ir.ReadPassword("Confirm export passphrase: ", stdout)
	if err != nil {
		return "", err
	}
	if p1 != p2 {
		return "", errors.New("passphrases did not match")
	}
	return p1, nil
}

// --- import (bundle path) --------------------------------------------------

// importBundle decrypts an export bundle and writes its secrets into the
// current vault, regardless of which backend produced it. Existing
// aliases are skipped unless overwrite is set.
func importBundle(data []byte, fromStdin bool, homeDir string, stdin io.Reader, stdout, stderr io.Writer, prefix, namespaceOverride string, overwrite bool) int {
	pass := os.Getenv(EnvExportPassphrase)
	if pass == "" {
		if fromStdin {
			fmt.Fprintf(stderr, "error: reading the bundle from stdin, so set %s (stdin cannot also carry the passphrase prompt)\n", EnvExportPassphrase)
			return 1
		}
		ir := auth.NewInputReader(stdin)
		p, err := ir.ReadPassword("Export passphrase: ", stdout)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		pass = p
	}

	plain, err := openExport(data, pass)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	var bundle exportBundle
	if err := json.Unmarshal(plain, &bundle); err != nil {
		fmt.Fprintf(stderr, "error: parsing bundle: %s\n", err)
		return 1
	}
	if bundle.Version > exportVersion {
		fmt.Fprintf(stderr, "error: bundle is schema version %d but this aql understands up to %d; upgrade aql\n", bundle.Version, exportVersion)
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
	krStdin := stdin
	if fromStdin {
		krStdin = nil
	}
	kr, err := openKeyring(s, homeDir, krStdin, stdout, "Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	// Bundle names are stored-form, so they are kept as-is — the default
	// namespace never silently rewrites them. --namespace explicitly
	// remaps every imported alias into the given namespace (":" = root);
	// --prefix applies to the base name, inside the namespace.
	remap := false
	remapNS := ""
	switch namespaceOverride {
	case "":
	case rootNamespaceRef:
		remap = true
	default:
		if !validNamespaceName(namespaceOverride) {
			fmt.Fprintf(stderr, "error: invalid namespace %q\n", namespaceOverride)
			return 1
		}
		remap, remapNS = true, namespaceOverride
	}

	imported, skipped := 0, 0
	for _, a := range bundle.Aliases {
		ns, base := splitAlias(a.Name)
		if remap {
			ns = remapNS
		}
		name := prefix + base
		if ns != "" {
			name = ns + ":" + name
		}
		if !validAlias(name) {
			fmt.Fprintf(stderr, "warning: skipping invalid alias %q\n", name)
			continue
		}
		if a.Value == "" {
			fmt.Fprintf(stderr, "warning: skipping %s (empty value)\n", name)
			continue
		}
		if existing, _ := s.FindAlias(name); existing != nil && !overwrite {
			fmt.Fprintf(stderr, "skipping existing alias %s (use --overwrite to replace)\n", name)
			skipped++
			continue
		}
		if err := kr.Set(name, a.Value); err != nil {
			fmt.Fprintf(stderr, "error: storing %s: %s\n", name, err)
			return 1
		}
		// Metadata namespace: derived from the final name; a root-level
		// name keeps the bundle's legacy tag unless root was an explicit
		// remap target.
		metaNS := ns
		if ns == "" && !remap {
			metaNS = a.Namespace
		}
		s.UpsertAlias(Alias{Name: name, Provider: a.Provider, Namespace: metaNS, Source: "import:bundle"})
		_ = appendAudit(homeDir, AuditEvent{
			Action: "vault.import", Alias: name, Provider: a.Provider,
			Outcome: "ok", Reason: "source=bundle",
		})
		fmt.Fprintf(stdout, "imported %s\n", name)
		imported++
	}
	if err := SaveStore(homeDir, s); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "imported %d secret(s)", imported)
	if skipped > 0 {
		fmt.Fprintf(stdout, ", skipped %d existing", skipped)
	}
	fmt.Fprintln(stdout)
	return 0
}

// --- bundle envelope crypto ------------------------------------------------

// sealExport encrypts plain under passphrase into the self-describing
// bundle envelope.
func sealExport(plain []byte, passphrase string) ([]byte, error) {
	salt := make([]byte, keyringSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key, err := scryptKey(passphrase, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	header := append([]byte(exportMagic), byte(exportEnvelopeFormat))
	aad := append(append([]byte{}, header...), salt...)
	ct := gcm.Seal(nil, nonce, plain, aad)
	out := make([]byte, 0, len(header)+len(salt)+len(nonce)+len(ct))
	out = append(out, header...)
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

// openExport validates and decrypts a bundle envelope. A newer envelope
// format than this binary understands is reported clearly.
func openExport(blob []byte, passphrase string) ([]byte, error) {
	magic := []byte(exportMagic)
	if len(blob) < len(magic)+1 || !bytes.Equal(blob[:len(magic)], magic) {
		return nil, errors.New("vault: not an aql vault export bundle")
	}
	format := int(blob[len(magic)])
	if format > exportEnvelopeFormat {
		return nil, fmt.Errorf("vault: export bundle is format %d but this aql understands up to %d; upgrade aql", format, exportEnvelopeFormat)
	}
	if format != 1 {
		return nil, fmt.Errorf("vault: unknown export bundle format %d", format)
	}
	off := len(magic) + 1
	if len(blob) < off+keyringSaltLen+keyringNonceLen+16 {
		return nil, errors.New("vault: export bundle is truncated")
	}
	salt := blob[off : off+keyringSaltLen]
	nonce := blob[off+keyringSaltLen : off+keyringSaltLen+keyringNonceLen]
	ct := blob[off+keyringSaltLen+keyringNonceLen:]
	key, err := scryptKey(passphrase, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	aad := append(append([]byte{}, blob[:off]...), salt...)
	plain, err := gcm.Open(nil, nonce, ct, aad)
	if err != nil {
		return nil, errors.New("vault: wrong export passphrase or corrupt bundle")
	}
	return plain, nil
}

// isTerminalWriter reports whether w is an interactive terminal, so the
// exporter can refuse to dump a binary bundle onto it.
func isTerminalWriter(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}
