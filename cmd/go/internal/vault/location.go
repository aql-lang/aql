package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Vault location customization.
//
// By default the vault lives in the folder {home}/.aql, with files
// named vault.<ext> — vault.jsonic (metadata), vault.keyring (the file
// backend), vault.lock, and vault.audit.jsonl. Two knobs override this,
// each settable either by a global flag placed before the mode or by an
// environment variable:
//
//	--folder PATH  / AQL_VAULT_FOLDER  put the vault in PATH instead of
//	                                   {home}/.aql.
//	--suffix NAME  / AQL_VAULT_SUFFIX  name the files vault.NAME.<ext>,
//	                                   so several vaults can share one
//	                                   folder without colliding.
//
// A flag, when given, wins over the matching environment variable: Run
// promotes the flag value into the environment so every resolver below
// reads a single source of truth.
const (
	// EnvFolder overrides the vault folder ({home}/.aql by default).
	EnvFolder = "AQL_VAULT_FOLDER"
	// EnvSuffix inserts an inner file-name segment: vault.<suffix>.jsonic.
	EnvSuffix = "AQL_VAULT_SUFFIX"
)

// vaultFolder returns the folder holding the vault's files: the
// AQL_VAULT_FOLDER override when set, otherwise {homeDir}/.aql. A
// relative override is resolved against the process working folder.
func vaultFolder(homeDir string) string {
	if f := os.Getenv(EnvFolder); f != "" {
		return f
	}
	return filepath.Join(homeDir, ".aql")
}

// vaultFileName builds one of the vault's inner file names for the
// given extension, inserting the configured suffix as
// vault.<suffix>.<ext> when AQL_VAULT_SUFFIX is set, otherwise
// vault.<ext>. ext is the part after the leading "vault.", e.g.
// "jsonic", "keyring", "lock", or "audit.jsonl".
func vaultFileName(ext string) string {
	if s := os.Getenv(EnvSuffix); s != "" {
		return "vault." + s + "." + ext
	}
	return "vault." + ext
}

// validSuffix accepts a file-name suffix: one segment of [A-Za-z0-9._-]
// that starts with a letter or digit, up to 64 chars. Requiring a
// leading alphanumeric and forbidding path separators keeps the suffix
// from escaping the vault folder or producing a hidden (dot-leading)
// file.
func validSuffix(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case (r == '.' || r == '_' || r == '-') && i > 0:
		default:
			return false
		}
	}
	return true
}

// splitLocationArgs pulls the global --folder/--suffix location flags
// out of args wherever they appear (in --flag=value or --flag value
// form, with one or two leading dashes), returning the remaining args —
// the mode and its own arguments — untouched. Everything from a bare
// "--" onward is passed through verbatim so a flag meant for a child
// process (vault exec ... -- cmd --folder=...) is never captured here.
func splitLocationArgs(args []string) (folder string, folderSet bool, suffix string, suffixSet bool, rest []string, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		name, val, hasVal, ok := matchLocationFlag(a)
		if !ok {
			rest = append(rest, a)
			continue
		}
		if !hasVal {
			if i+1 >= len(args) {
				return "", false, "", false, nil, fmt.Errorf("flag --%s needs a value", name)
			}
			i++
			val = args[i]
		}
		switch name {
		case "folder":
			folder, folderSet = val, true
		case "suffix":
			suffix, suffixSet = val, true
		}
	}
	return folder, folderSet, suffix, suffixSet, rest, nil
}

// matchLocationFlag recognizes -folder/--folder and -suffix/--suffix,
// each optionally with an attached =value. ok is false for any other
// argument (which the caller passes through to the mode unchanged).
func matchLocationFlag(a string) (name, val string, hasVal, ok bool) {
	if !strings.HasPrefix(a, "-") {
		return "", "", false, false
	}
	// Strip one or two leading dashes, then split an attached value.
	body := strings.TrimPrefix(strings.TrimPrefix(a, "-"), "-")
	flagName, flagVal, hasEq := strings.Cut(body, "=")
	switch flagName {
	case "folder", "suffix":
		return flagName, flagVal, hasEq, true
	}
	return "", "", false, false
}
