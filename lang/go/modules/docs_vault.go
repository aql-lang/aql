package modules

func init() {
	// `identity` and `reveal` are the pair a reader most needs told
	// apart, and no signature line can tell them apart: both take an
	// alias String, and only the examples show that one hands back a
	// secret and the other deliberately does not.
	registerExamples("aql:vault", map[string][]string{
		"identity": {
			`def cred (Vault.identity "acme-mtls")                   ;# <vault identity> — opaque, redacts`,
			`Net.fetch "https://internal.example.com" {tls: {identity: cred}}`,
			`Net.connect-raw {tcp: "db.internal:5432"  tls: {identity: cred}}`,
			`;# The vault is read INSIDE the handshake, so a rotated secret is`,
			`;# picked up without re-running, and a program that never dials`,
			`;# never reveals anything. There is no path from the handle back`,
			`;# to the key — that is the difference from Vault.reveal.`,
		},
		"reveal": {
			`def token (Vault.reveal "github-pat")                   ;# a String — the actual secret`,
			`Net.fetch "https://api.github.com/user" {`,
			`  headers: {authorization: (join "" ["Bearer " token])}`,
			`}`,
			`;# Right for a token a program must paste into a header. For a`,
			`;# PRIVATE KEY use Vault.identity instead: the moment a key is a`,
			`;# guest String it can be printed, logged, or sent somewhere.`,
		},
	})

	registerDocs("aql:vault", map[string]string{
		"identity":             "An opaque handle to a vault-held TLS client credential, for `tls: {identity: …}`. Unlike `reveal` it never yields the secret as a value.",
		"status":               "The active vault's status map: {ok backend locked aliases …} from the host backend.",
		"needs-passphrase":     "Whether the active vault requires a passphrase that has not been collected this session.",
		"authed":               "Whether the host adapter holds a verified passphrase for the active vault.",
		"authenticate":         "Verify a passphrase against the active vault; the host caches it for the session on success.",
		"status-text":          "The vault status panel as the CLI renders it.",
		"providers-text":       "The known-provider catalogue as the CLI renders it.",
		"config-text":          "The vault config listing as the CLI renders it.",
		"secrets":              "The stored secrets' metadata rows (aliases only — never values).",
		"secret":               "One secret's metadata map by alias, or None when the alias is unknown.",
		"reveal":               "The secret value for an alias (gated by the vault policy's reveal op).",
		"add":                  "Store a secret: {alias value provider namespace expiry}.",
		"rotate":               "Replace a secret's value: {alias value revoke-caps expiry}.",
		"remove":               "Delete a secret by alias.",
		"rename":               "Rename a secret: {from to revoke-caps}.",
		"set-expiry":           "Set an alias's expiry reminder.",
		"clear-expiry":         "Clear an alias's expiry reminder.",
		"recipes":              "The provider-aware exec inject commands for an alias, as {cmd desc} rows.",
		"capabilities":         "The capability tokens map: {caps active}.",
		"grant":                "Grant a capability token: {alias agent hosts methods ttl …}; returns the one-time token output.",
		"revoke":               "Revoke a capability token by id.",
		"passwords":            "The scoped vault password slots.",
		"password-add":         "Add a scoped vault password: {name pass scope namespaces ttl}.",
		"password-remove":      "Remove a vault password slot by name.",
		"password-revoke-temp": "Revoke every temporary vault password.",
		"temp-password-count":  "How many temporary vault passwords are active.",
		"verify":               "Verify the vault's integrity ({prune: true} prunes); returns {text ok}.",
		"scan":                 "Scan paths for leaked secret values; returns the report text.",
		"history":              "The vault's generation history as the CLI renders it.",
		"restore":              "Restore the vault to a generation number.",
		"audit":                "The audit log as the CLI renders it, filtered by {filters}.",
		"config-set":           "Set a vault config key.",
		"config-unset":         "Unset a vault config key.",
		"lock":                 "Lock the vault.",
		"unlock":               "Unlock the vault.",
		"locked":               "Whether the vault is locked.",
		"prefs":                "The host TUI preferences map (e.g. theme).",
		"set-prefs":            "Persist host TUI preferences: {theme}.",
		"vaults":               "The known-vault index rows (locations only — never secrets).",
		"switch":               "Make another vault active: {folder suffix}.",
		"create":               "Create a new vault: {folder suffix backend pass}.",
		"set-default":          "Mark the active vault as the index default.",
		"prune-index":          "Drop a vault from the index: {folder suffix}.",
		"copy":                 "Copy text to the system clipboard; returns the clipboard tool's label.",
	})
}
