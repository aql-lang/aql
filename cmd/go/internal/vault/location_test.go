package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidSuffix(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"work", true},
		{"v2", true},
		{"a.b-c_d", true},
		{strings.Repeat("a", 64), true},
		{"", false},
		{".hidden", false}, // leading dot
		{"-lead", false},   // leading dash
		{"a/b", false},     // path separator
		{"a:b", false},     // colon
		{"a b", false},     // space
		{strings.Repeat("a", 65), false},
	}
	for _, c := range cases {
		if got := validSuffix(c.in); got != c.want {
			t.Errorf("validSuffix(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestFolderTildeExpandedViaFlag(t *testing.T) {
	home := testHome(t)

	// A ~ that the shell did not expand (e.g. from --folder=~/sub) must
	// resolve under the home folder, not a literal "~" directory.
	if code, _, errOut := runVault(t, "", "--folder", "~/sub", "init", "--backend=file"); code != 0 {
		t.Fatalf("init: %s", errOut)
	}
	if _, err := os.Stat(filepath.Join(home, "sub", "vault.jsonic")); err != nil {
		t.Errorf("store not under expanded home: %v", err)
	}
	if _, err := os.Stat(filepath.Join("~", "sub")); err == nil {
		t.Errorf("a literal ~ folder was created")
	}
}

func TestSplitLocationArgs(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		folder    string
		folderSet bool
		suffix    string
		suffixSet bool
		rest      []string
		wantErr   bool
	}{
		{name: "none", args: []string{"init"}, rest: []string{"init"}},
		{name: "folder eq before mode", args: []string{"--folder=/x", "init"}, folder: "/x", folderSet: true, rest: []string{"init"}},
		{name: "folder space before mode", args: []string{"--folder", "/x", "init"}, folder: "/x", folderSet: true, rest: []string{"init"}},
		{name: "single dash", args: []string{"-folder=/x", "status"}, folder: "/x", folderSet: true, rest: []string{"status"}},
		{name: "suffix space", args: []string{"--suffix", "work", "add", "k"}, suffix: "work", suffixSet: true, rest: []string{"add", "k"}},
		{name: "flag after mode", args: []string{"init", "--folder=/x"}, folder: "/x", folderSet: true, rest: []string{"init"}},
		{name: "both", args: []string{"--folder=/x", "--suffix=w", "list"}, folder: "/x", folderSet: true, suffix: "w", suffixSet: true, rest: []string{"list"}},
		{name: "passthrough other flags", args: []string{"add", "--from-stdin", "k"}, rest: []string{"add", "--from-stdin", "k"}},
		{name: "stop at double dash", args: []string{"exec", "k", "--", "cmd", "--folder=child"}, rest: []string{"exec", "k", "--", "cmd", "--folder=child"}},
		{name: "missing value", args: []string{"--folder"}, wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			folder, folderSet, suffix, suffixSet, rest, err := splitLocationArgs(c.args)
			if c.wantErr {
				if err == nil {
					t.Fatalf("want error, got rest=%v", rest)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if folder != c.folder || folderSet != c.folderSet || suffix != c.suffix || suffixSet != c.suffixSet {
				t.Errorf("got folder=%q,%v suffix=%q,%v; want folder=%q,%v suffix=%q,%v",
					folder, folderSet, suffix, suffixSet, c.folder, c.folderSet, c.suffix, c.suffixSet)
			}
			if strings.Join(rest, "\x00") != strings.Join(c.rest, "\x00") {
				t.Errorf("rest = %v, want %v", rest, c.rest)
			}
		})
	}
}

func TestCustomFolderViaEnv(t *testing.T) {
	home := testHome(t)
	folder := filepath.Join(t.TempDir(), "myvault")
	t.Setenv(EnvFolder, folder)

	if code, _, errOut := runVault(t, "", "init", "--backend=file"); code != 0 {
		t.Fatalf("init: %s", errOut)
	}
	// The store and keyring land in the custom folder.
	if _, err := os.Stat(filepath.Join(folder, "vault.jsonic")); err != nil {
		t.Errorf("store not in custom folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(folder, "vault.keyring")); err != nil {
		t.Errorf("keyring not in custom folder: %v", err)
	}
	// The default ~/.aql is left untouched.
	if _, err := os.Stat(filepath.Join(home, ".aql", "vault.jsonic")); err == nil {
		t.Errorf("store wrongly created under the default folder")
	}
	// StorePath reflects the override.
	if got := StorePath(home); got != filepath.Join(folder, "vault.jsonic") {
		t.Errorf("StorePath = %q, want under %q", got, folder)
	}
}

func TestCustomSuffixViaEnv(t *testing.T) {
	home := testHome(t)
	t.Setenv(EnvSuffix, "work")

	if code, _, errOut := runVault(t, "", "init", "--backend=file"); code != 0 {
		t.Fatalf("init: %s", errOut)
	}
	base := filepath.Join(home, ".aql")
	if _, err := os.Stat(filepath.Join(base, "vault.work.jsonic")); err != nil {
		t.Errorf("suffixed store missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "vault.work.keyring")); err != nil {
		t.Errorf("suffixed keyring missing: %v", err)
	}
	// The unsuffixed default name must not exist.
	if _, err := os.Stat(filepath.Join(base, "vault.jsonic")); err == nil {
		t.Errorf("unsuffixed store wrongly created")
	}
	// A second vault with a different suffix coexists in the same folder.
	t.Setenv(EnvSuffix, "play")
	if code, _, errOut := runVault(t, "", "init", "--backend=file"); code != 0 {
		t.Fatalf("init play: %s", errOut)
	}
	if _, err := os.Stat(filepath.Join(base, "vault.play.jsonic")); err != nil {
		t.Errorf("second suffixed store missing: %v", err)
	}
}

func TestCustomLocationViaFlags(t *testing.T) {
	testHome(t)
	folder := filepath.Join(t.TempDir(), "flagvault")

	// Global flags precede the mode and set up a fresh vault.
	if code, _, errOut := runVault(t, "", "--folder", folder, "--suffix=team", "init", "--backend=file"); code != 0 {
		t.Fatalf("init via flags: %s", errOut)
	}
	want := filepath.Join(folder, "vault.team.jsonic")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("flag-located store missing at %s: %v", want, err)
	}

	// A subsequent command targeting the same vault (via the same flags)
	// can read it back.
	if code, _, errOut := runVault(t, "sk\n", "--folder", folder, "--suffix=team", "add", "--from-stdin", "k"); code != 0 {
		t.Fatalf("add via flags: %s", errOut)
	}
	code, out, _ := runVault(t, "", "--folder", folder, "--suffix=team", "list")
	if code != 0 || !strings.Contains(out, "k") {
		t.Errorf("list via flags missing alias: %q", out)
	}
}

func TestFlagOverridesEnvFolder(t *testing.T) {
	testHome(t)
	envFolder := filepath.Join(t.TempDir(), "from-env")
	flagFolder := filepath.Join(t.TempDir(), "from-flag")
	t.Setenv(EnvFolder, envFolder)

	if code, _, errOut := runVault(t, "", "--folder", flagFolder, "init", "--backend=file"); code != 0 {
		t.Fatalf("init: %s", errOut)
	}
	if _, err := os.Stat(filepath.Join(flagFolder, "vault.jsonic")); err != nil {
		t.Errorf("flag folder not used: %v", err)
	}
	if _, err := os.Stat(filepath.Join(envFolder, "vault.jsonic")); err == nil {
		t.Errorf("env folder used despite flag override")
	}
}

func TestInvalidSuffixRejected(t *testing.T) {
	testHome(t)
	code, _, errOut := runVault(t, "", "--suffix=bad/slash", "status")
	if code == 0 {
		t.Fatal("expected non-zero exit for invalid suffix")
	}
	if !strings.Contains(errOut, "invalid vault suffix") {
		t.Errorf("missing invalid-suffix message: %q", errOut)
	}
}
