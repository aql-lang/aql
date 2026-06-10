package vault

import (
	"errors"
	"testing"
)

// fakeLookPath returns a lookPath that reports the named binaries as
// present and everything else as missing.
func fakeLookPath(present ...string) func(string) (string, error) {
	set := map[string]bool{}
	for _, p := range present {
		set[p] = true
	}
	return func(bin string) (string, error) {
		if set[bin] {
			return "/usr/bin/" + bin, nil
		}
		return "", errors.New("not found")
	}
}

func fakeGetenv(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

func TestDetectClipboard(t *testing.T) {
	tests := []struct {
		name      string
		env       clipEnv
		wantLabel string
		wantPaste string // pasteArgv[0]
		wantErr   bool
	}{
		{
			name:      "darwin pb tools",
			env:       clipEnv{goos: "darwin", getenv: fakeGetenv(nil), lookPath: fakeLookPath("pbpaste", "pbcopy")},
			wantLabel: "pbpaste/pbcopy", wantPaste: "pbpaste",
		},
		{
			name:    "darwin missing tools",
			env:     clipEnv{goos: "darwin", getenv: fakeGetenv(nil), lookPath: fakeLookPath()},
			wantErr: true,
		},
		{
			name:      "windows powershell",
			env:       clipEnv{goos: "windows", getenv: fakeGetenv(nil), lookPath: fakeLookPath("powershell.exe")},
			wantLabel: "powershell.exe Get-/Set-Clipboard", wantPaste: "powershell.exe",
		},
		{
			name:      "windows pwsh fallback",
			env:       clipEnv{goos: "windows", getenv: fakeGetenv(nil), lookPath: fakeLookPath("pwsh")},
			wantPaste: "pwsh",
		},
		{
			name:    "windows none",
			env:     clipEnv{goos: "windows", getenv: fakeGetenv(nil), lookPath: fakeLookPath()},
			wantErr: true,
		},
		{
			name:      "linux wayland session",
			env:       clipEnv{goos: "linux", getenv: fakeGetenv(map[string]string{"WAYLAND_DISPLAY": "wayland-0"}), lookPath: fakeLookPath("wl-paste", "wl-copy", "xclip")},
			wantLabel: "wl-paste/wl-copy", wantPaste: "wl-paste",
		},
		{
			name:      "linux x11 prefers xclip when DISPLAY set",
			env:       clipEnv{goos: "linux", getenv: fakeGetenv(map[string]string{"DISPLAY": ":0"}), lookPath: fakeLookPath("wl-paste", "wl-copy", "xclip")},
			wantLabel: "xclip", wantPaste: "xclip",
		},
		{
			name:      "linux wl-clipboard with no display still used",
			env:       clipEnv{goos: "linux", getenv: fakeGetenv(nil), lookPath: fakeLookPath("wl-paste", "wl-copy")},
			wantLabel: "wl-paste/wl-copy", wantPaste: "wl-paste",
		},
		{
			name:      "linux xsel",
			env:       clipEnv{goos: "linux", getenv: fakeGetenv(map[string]string{"DISPLAY": ":0"}), lookPath: fakeLookPath("xsel")},
			wantLabel: "xsel", wantPaste: "xsel",
		},
		{
			name:    "linux none",
			env:     clipEnv{goos: "linux", getenv: fakeGetenv(nil), lookPath: fakeLookPath()},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := detectClipboard(tc.env)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if tc.wantLabel != "" && got.label != tc.wantLabel {
				t.Errorf("label = %q, want %q", got.label, tc.wantLabel)
			}
			if len(got.pasteArgv) == 0 || got.pasteArgv[0] != tc.wantPaste {
				t.Errorf("pasteArgv = %v, want first element %q", got.pasteArgv, tc.wantPaste)
			}
			if len(got.clearArgv) == 0 {
				t.Errorf("clearArgv is empty for %s", tc.name)
			}
		})
	}
}
