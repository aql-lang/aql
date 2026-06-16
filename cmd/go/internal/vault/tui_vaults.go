package vault

// tui_vaults.go — the vault index (~/.aql/vaults.jsonic) and discovery.
//
// A machine can hold several vaults: the default ~/.aql/vault.jsonic,
// suffixed siblings vault.<suffix>.jsonic sharing a folder, and vaults in
// arbitrary --folder locations. Vaults in a known folder are found by
// globbing vault*.jsonic; vaults in an arbitrary --folder are NOT findable
// without a record, so `vault init` writes a small index here that the TUI
// (and `vault folder`) read to enumerate every known vault regardless of
// where it lives.
//
// The index records LOCATIONS ONLY — folder, suffix, a label, and
// timestamps — never any secret, key, or value. It lives in the home
// ~/.aql folder (not the --folder override) so it is a single global
// registry across all vault locations.

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aql-lang/aql/cmd/go/internal/pathutil"
)

// vaultIndexVersion is the on-disk schema version of vaults.jsonic.
const vaultIndexVersion = 1

// vaultIndexFile is the index file name within the home ~/.aql folder.
const vaultIndexFile = "vaults.jsonic"

// vaultRef is one known vault location. It carries no secret material:
// only where the vault lives plus display/ordering hints.
type vaultRef struct {
	Folder       string `json:"folder"`                   // folder holding the vault files
	Suffix       string `json:"suffix,omitempty"`         // "" == default vault.jsonic
	Label        string `json:"label,omitempty"`          // optional human label
	Backend      string `json:"backend,omitempty"`        // recorded at init, for display only
	CreatedAt    string `json:"created_at,omitempty"`     // RFC3339
	LastOpenedAt string `json:"last_opened_at,omitempty"` // RFC3339
	Default      bool   `json:"default,omitempty"`        // preferred vault at launch
}

// vaultIndex is the parsed vaults.jsonic file.
type vaultIndex struct {
	Version int        `json:"version"`
	Vaults  []vaultRef `json:"vaults,omitempty"`
}

// discoveredVault is one row in the enumerated vault list: an index ref
// merged with what the filesystem reports.
type discoveredVault struct {
	vaultRef
	Exists bool // the vault.jsonic metadata file is present
	Locked bool // store.Locked (best-effort; false when unreadable)
	InScan bool // found by globbing a folder
	InIdx  bool // present in the index
}

// homeAQLDir is the home ~/.aql folder, where the global index lives. It
// is independent of AQL_VAULT_FOLDER so the registry stays single even
// when individual vaults live elsewhere.
func homeAQLDir(homeDir string) string { return filepath.Join(homeDir, ".aql") }

// vaultIndexPath is the path to the index file.
func vaultIndexPath(homeDir string) string {
	return filepath.Join(homeAQLDir(homeDir), vaultIndexFile)
}

// metaFileName builds the metadata file name for a suffix, independent of
// the AQL_VAULT_SUFFIX env (unlike vaultFileName): "" -> vault.jsonic,
// "work" -> vault.work.jsonic.
func metaFileName(suffix string) string {
	if suffix == "" {
		return "vault.jsonic"
	}
	return "vault." + suffix + ".jsonic"
}

// suffixFromMetaName extracts the suffix from a metadata file name, or
// reports ok=false if the name is not a vault metadata file. "vault.jsonic"
// -> ("", true); "vault.work.jsonic" -> ("work", true); anything else (incl.
// vault.jsonic.sig / .log) -> ("", false).
func suffixFromMetaName(name string) (string, bool) {
	if name == "vault.jsonic" {
		return "", true
	}
	if !strings.HasPrefix(name, "vault.") || !strings.HasSuffix(name, ".jsonic") {
		return "", false
	}
	mid := name[len("vault.") : len(name)-len(".jsonic")]
	if mid == "" || !validSuffix(mid) {
		return "", false
	}
	return mid, true
}

// loadVaultIndex reads and parses vaults.jsonic. A missing file is not an
// error — it returns an empty index. A corrupt file returns an error so
// callers can decide to warn-and-continue rather than crash.
func loadVaultIndex(homeDir string) (*vaultIndex, error) {
	data, err := os.ReadFile(vaultIndexPath(homeDir))
	if err != nil {
		if os.IsNotExist(err) {
			return &vaultIndex{Version: vaultIndexVersion}, nil
		}
		return nil, err
	}
	idx := &vaultIndex{}
	if err := json.Unmarshal(data, idx); err != nil {
		return nil, fmt.Errorf("vault: parsing %s: %w", vaultIndexPath(homeDir), err)
	}
	if idx.Version == 0 {
		idx.Version = vaultIndexVersion
	}
	return idx, nil
}

// saveVaultIndex writes vaults.jsonic atomically with mode 0600 (the home
// ~/.aql folder is created 0700 if absent).
func saveVaultIndex(homeDir string, idx *vaultIndex) error {
	if homeDir == "" {
		// No home means no global registry; silently skip rather than
		// writing to an unexpected location.
		return nil
	}
	idx.Version = vaultIndexVersion
	if err := os.MkdirAll(homeAQLDir(homeDir), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(vaultIndexPath(homeDir), data, 0600)
}

// find returns the ref matching (folder, suffix) and its position, or
// (nil, -1). Folders are compared after cleaning so /a/b and /a/b/ match.
func (idx *vaultIndex) find(folder, suffix string) (*vaultRef, int) {
	folder = filepath.Clean(folder)
	for i := range idx.Vaults {
		if filepath.Clean(idx.Vaults[i].Folder) == folder && idx.Vaults[i].Suffix == suffix {
			return &idx.Vaults[i], i
		}
	}
	return nil, -1
}

// upsert inserts ref or updates the existing entry for its (folder,
// suffix), preserving CreatedAt and only overwriting non-empty fields.
func (idx *vaultIndex) upsert(ref vaultRef) {
	ref.Folder = filepath.Clean(ref.Folder)
	if existing, _ := idx.find(ref.Folder, ref.Suffix); existing != nil {
		if ref.Label != "" {
			existing.Label = ref.Label
		}
		if ref.Backend != "" {
			existing.Backend = ref.Backend
		}
		if ref.CreatedAt != "" && existing.CreatedAt == "" {
			existing.CreatedAt = ref.CreatedAt
		}
		if ref.LastOpenedAt != "" {
			existing.LastOpenedAt = ref.LastOpenedAt
		}
		if ref.Default {
			existing.Default = true
		}
		return
	}
	idx.Vaults = append(idx.Vaults, ref)
}

// remove drops the entry for (folder, suffix). Returns true if present.
func (idx *vaultIndex) remove(folder, suffix string) bool {
	if _, i := idx.find(folder, suffix); i >= 0 {
		idx.Vaults = append(idx.Vaults[:i], idx.Vaults[i+1:]...)
		return true
	}
	return false
}

// clearDefaults unmarks every default entry. setDefaultVault uses it so at
// most one vault is the default at a time.
func (idx *vaultIndex) clearDefaults() {
	for i := range idx.Vaults {
		idx.Vaults[i].Default = false
	}
}

// nowRFC3339 is the timestamp format used across the vault package.
func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

// recordVaultInit records a freshly-created vault in the index. Best-effort:
// a registry write must never fail the init itself, so callers ignore the
// error (or log it).
func recordVaultInit(homeDir, folder, suffix, backend string) error {
	idx, err := loadVaultIndex(homeDir)
	if err != nil {
		// A corrupt index shouldn't block recording; start fresh.
		idx = &vaultIndex{Version: vaultIndexVersion}
	}
	idx.upsert(vaultRef{
		Folder:    folder,
		Suffix:    suffix,
		Backend:   backend,
		CreatedAt: nowRFC3339(),
	})
	return saveVaultIndex(homeDir, idx)
}

// registerVault records an already-existing vault in the index (unlike
// recordVaultInit, it does not stamp CreatedAt, since the vault predates the
// registration). Used by `vault vaults add`.
func registerVault(homeDir, folder, suffix, backend string) error {
	idx, err := loadVaultIndex(homeDir)
	if err != nil {
		return err
	}
	idx.upsert(vaultRef{Folder: folder, Suffix: suffix, Backend: backend})
	return saveVaultIndex(homeDir, idx)
}

// recordVaultOpened stamps LastOpenedAt for a vault, adding an entry if the
// vault was discovered by scan but not yet in the index. Best-effort.
func recordVaultOpened(homeDir, folder, suffix string) error {
	idx, err := loadVaultIndex(homeDir)
	if err != nil {
		idx = &vaultIndex{Version: vaultIndexVersion}
	}
	idx.upsert(vaultRef{Folder: folder, Suffix: suffix, LastOpenedAt: nowRFC3339()})
	return saveVaultIndex(homeDir, idx)
}

// setDefaultVault marks (folder, suffix) as the launch default, clearing any
// previous default. The vault is added to the index if missing.
func setDefaultVault(homeDir, folder, suffix string) error {
	idx, err := loadVaultIndex(homeDir)
	if err != nil {
		idx = &vaultIndex{Version: vaultIndexVersion}
	}
	idx.clearDefaults()
	idx.upsert(vaultRef{Folder: folder, Suffix: suffix, Default: true})
	return saveVaultIndex(homeDir, idx)
}

// pruneVaultEntry removes a (folder, suffix) from the index. Used to drop a
// stale entry whose metadata file no longer exists.
func pruneVaultEntry(homeDir, folder, suffix string) error {
	idx, err := loadVaultIndex(homeDir)
	if err != nil {
		return err
	}
	if !idx.remove(folder, suffix) {
		return nil
	}
	return saveVaultIndex(homeDir, idx)
}

// peekStore reads just the backend + locked flag from a vault's metadata
// file without authenticating. ok is false when the file is absent or
// unparseable. It deliberately avoids LoadStore (which is env-path coupled)
// so any folder/suffix can be inspected.
func peekStore(folder, suffix string) (backend string, locked, ok bool) {
	data, err := os.ReadFile(filepath.Join(folder, metaFileName(suffix)))
	if err != nil {
		return "", false, false
	}
	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		return "", false, false
	}
	return s.Backend, s.Locked, true
}

// scanFolderVaults globs a folder for vault*.jsonic metadata files and
// returns the suffixes found (incl. "" for the default vault). A missing
// folder yields no entries and no error.
func scanFolderVaults(folder string) []string {
	matches, err := filepath.Glob(filepath.Join(folder, "vault*.jsonic"))
	if err != nil {
		return nil
	}
	var suffixes []string
	for _, m := range matches {
		if suffix, ok := suffixFromMetaName(filepath.Base(m)); ok {
			suffixes = append(suffixes, suffix)
		}
	}
	return suffixes
}

// enumerateVaults lists every known vault: the union of (a) vault*.jsonic
// files globbed from the home ~/.aql folder, the currently-resolved vault
// folder, and every folder named in the index, with (b) the index entries
// themselves. Results are deduped by (cleaned folder, suffix), annotated
// with existence/lock state, and sorted default-first then
// most-recently-opened.
func enumerateVaults(homeDir string) ([]discoveredVault, error) {
	idx, err := loadVaultIndex(homeDir)
	if err != nil {
		// Treat a corrupt index as empty so discovery still surfaces
		// filesystem vaults; the caller may warn separately.
		idx = &vaultIndex{Version: vaultIndexVersion}
	}

	// Folders to scan: home ~/.aql, the resolved vault folder (honors an
	// AQL_VAULT_FOLDER override active for this process), and every index
	// folder.
	folderSet := map[string]bool{}
	addFolder := func(f string) {
		if f != "" {
			folderSet[filepath.Clean(f)] = true
		}
	}
	addFolder(homeAQLDir(homeDir))
	addFolder(vaultFolder(homeDir))
	for _, ref := range idx.Vaults {
		addFolder(ref.Folder)
	}

	// key (cleaned folder + "\x00" + suffix) -> position in out.
	out := []discoveredVault{}
	pos := map[string]int{}
	key := func(folder, suffix string) string { return filepath.Clean(folder) + "\x00" + suffix }

	for folder := range folderSet {
		for _, suffix := range scanFolderVaults(folder) {
			k := key(folder, suffix)
			backend, locked, _ := peekStore(folder, suffix)
			dv := discoveredVault{
				vaultRef: vaultRef{Folder: filepath.Clean(folder), Suffix: suffix, Backend: backend},
				Exists:   true,
				Locked:   locked,
				InScan:   true,
			}
			pos[k] = len(out)
			out = append(out, dv)
		}
	}

	// Merge index metadata onto scanned rows, and add index-only rows
	// (possibly stale: file missing).
	for _, ref := range idx.Vaults {
		k := key(ref.Folder, ref.Suffix)
		if i, ok := pos[k]; ok {
			out[i].InIdx = true
			out[i].Label = ref.Label
			out[i].CreatedAt = ref.CreatedAt
			out[i].LastOpenedAt = ref.LastOpenedAt
			out[i].Default = ref.Default
			if out[i].Backend == "" {
				out[i].Backend = ref.Backend
			}
			continue
		}
		backend, locked, exists := peekStore(ref.Folder, ref.Suffix)
		dv := discoveredVault{vaultRef: ref, Exists: exists, Locked: locked, InIdx: true}
		dv.Folder = filepath.Clean(ref.Folder)
		if backend != "" {
			dv.Backend = backend
		}
		pos[k] = len(out)
		out = append(out, dv)
	}

	sortDiscoveredVaults(out)
	return out, nil
}

// sortDiscoveredVaults orders vaults default-first, then most-recently
// opened, then by folder and suffix for stability.
func sortDiscoveredVaults(vs []discoveredVault) {
	sort.SliceStable(vs, func(i, j int) bool {
		a, b := vs[i], vs[j]
		if a.Default != b.Default {
			return a.Default
		}
		if a.LastOpenedAt != b.LastOpenedAt {
			return a.LastOpenedAt > b.LastOpenedAt // newer first
		}
		if a.Folder != b.Folder {
			return a.Folder < b.Folder
		}
		return a.Suffix < b.Suffix
	})
}

// vaultDisplayName renders a short, friendly name for a vault row: the
// label if set, else the suffix, else "(default)".
func (dv discoveredVault) displayName() string {
	switch {
	case dv.Label != "":
		return dv.Label
	case dv.Suffix != "":
		return dv.Suffix
	default:
		return "(default)"
	}
}

// runVaults implements `aql vault folder` (aliases: folders, vaults) and its
// subcommands:
//
//	vault folder                      list discovered + recorded vaults
//	vault [--suffix=NAME] folder add <dir>   register an existing vault
//	vault [--suffix=NAME] folder rm  <dir>   forget a vault (index only)
func runVaults(args []string, homeDir string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "add", "register":
			return runVaultsAdd(args[1:], homeDir, stdout, stderr)
		case "rm", "remove", "forget":
			return runVaultsRemove(args[1:], homeDir, stdout, stderr)
		}
	}
	return runVaultsList(args, homeDir, stdout, stderr)
}

// runVaultsList prints the discovered + recorded vaults so the index is
// useful outside the TUI.
func runVaultsList(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault vaults", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	vs, err := enumerateVaults(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if len(vs) == 0 {
		fmt.Fprintln(stdout, "vault: no vaults found (run `aql vault init`)")
		return 0
	}
	fmt.Fprintf(stdout, "%-2s %-16s %-10s %-10s %-8s %s\n", "", "NAME", "SUFFIX", "BACKEND", "STATUS", "FOLDER")
	for _, dv := range vs {
		marker := " "
		if dv.Default {
			marker = "*"
		}
		status := "ok"
		switch {
		case !dv.Exists:
			status = "missing"
		case dv.Locked:
			status = "locked"
		}
		fmt.Fprintf(stdout, "%-2s %-16s %-10s %-10s %-8s %s\n",
			marker, dv.displayName(), dash(dv.Suffix), dash(dv.Backend), status, dv.Folder)
	}
	return 0
}

// runVaultsAdd registers an already-existing vault into the index so it is
// discoverable by the TUI and `vault vaults`. The suffix comes from the
// global --suffix location flag (e.g. `aql vault --suffix=work vaults add
// /path`); when omitted it is auto-detected if the folder holds exactly one
// vault.
func runVaultsAdd(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault vaults add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "error: usage: aql vault [--suffix=NAME] folder add <dir>")
		return 1
	}
	folder := filepath.Clean(pathutil.ExpandTilde(fs.Arg(0), homeDir))

	// The global --suffix was promoted to EnvSuffix by Run; an empty value
	// means "auto-detect".
	sfx := os.Getenv(EnvSuffix)
	if sfx == "" {
		found := scanFolderVaults(folder)
		switch len(found) {
		case 0:
			fmt.Fprintf(stderr, "error: no vault found in %s (expected a vault*.jsonic file)\n", folder)
			return 1
		case 1:
			sfx = found[0]
		default:
			sort.Strings(found)
			fmt.Fprintf(stderr, "error: %s holds multiple vaults; pick one with --suffix (found: %s)\n", folder, strings.Join(quoteSuffixes(found), ", "))
			return 1
		}
	}

	backend, _, ok := peekStore(folder, sfx)
	if !ok {
		fmt.Fprintf(stderr, "error: no vault metadata at %s\n", filepath.Join(folder, metaFileName(sfx)))
		return 1
	}
	if err := registerVault(homeDir, folder, sfx, backend); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "registered vault %s (suffix=%s, backend=%s) at %s\n", labelOrDefault(sfx), dash(sfx), backend, folder)
	return 0
}

// runVaultsRemove forgets a vault from the index (the vault files are left
// untouched). The suffix comes from the global --suffix location flag.
func runVaultsRemove(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault vaults rm", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "error: usage: aql vault [--suffix=NAME] folder rm <dir>")
		return 1
	}
	folder := filepath.Clean(pathutil.ExpandTilde(fs.Arg(0), homeDir))
	suffix := os.Getenv(EnvSuffix)
	idx, err := loadVaultIndex(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if !idx.remove(folder, suffix) {
		fmt.Fprintf(stderr, "error: no index entry for %s (suffix=%q)\n", folder, suffix)
		return 1
	}
	if err := saveVaultIndex(homeDir, idx); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "forgot vault at %s\n", folder)
	return 0
}

// labelOrDefault renders a suffix as a display name, "" -> "(default)".
func labelOrDefault(suffix string) string {
	if suffix == "" {
		return "(default)"
	}
	return suffix
}

// quoteSuffixes renders suffixes for an error message, mapping "" to ":".
func quoteSuffixes(suffixes []string) []string {
	out := make([]string, len(suffixes))
	for i, s := range suffixes {
		if s == "" {
			out[i] = "(default)"
		} else {
			out[i] = s
		}
	}
	return out
}

// launchVault picks the vault to open when `aql vault -i` is invoked with no
// explicit --folder/--suffix: the index default, else the only vault, else
// the most-recently-opened. ok is false when there is nothing to open (zero
// discovered vaults) so the caller drops into the picker/create flow.
func launchVault(homeDir string) (folder, suffix string, ok bool) {
	vs, err := enumerateVaults(homeDir)
	if err != nil || len(vs) == 0 {
		return "", "", false
	}
	// enumerateVaults already sorts default-first then most-recent; prefer
	// an existing (non-stale) vault.
	for _, dv := range vs {
		if dv.Exists {
			return dv.Folder, dv.Suffix, true
		}
	}
	return "", "", false
}
