package vault

// confirm_seam7_test.go — seam-7 coverage for confirm.go's interactive
// (TTY) branch, driven by forcing the isTerminal seam true and feeding an
// *os.File stdin.

import (
	"bytes"
	"os"
	"testing"
)

// c7forceTTY makes stdinIsTTY report true for any *os.File.
func c7forceTTY(t *testing.T) {
	t.Helper()
	old := isTerminal
	isTerminal = func(int) bool { return true }
	t.Cleanup(func() { isTerminal = old })
}

// c7ttyFile returns a rewound *os.File preloaded with content (empty
// content yields an immediate-EOF reader that makes ReadLine error).
func c7ttyFile(t *testing.T, content string) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "c7tty")
	if err != nil {
		t.Fatal(err)
	}
	if content != "" {
		if _, err := f.WriteString(content); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestC7ConfirmDestructiveTTY(t *testing.T) {
	c7forceTTY(t)
	var out bytes.Buffer

	// tier2: exact target confirms.
	if err := confirmDestructive(confirmTier2, "TARGET", "wipe it", false, false, c7ttyFile(t, "TARGET\n"), &out); err != nil {
		t.Errorf("tier2 match = %v", err)
	}
	// tier2: wrong token aborts.
	if err := confirmDestructive(confirmTier2, "TARGET", "wipe it", false, false, c7ttyFile(t, "nope\n"), &out); err != errAborted {
		t.Errorf("tier2 mismatch = %v, want errAborted", err)
	}
	// tier2: no input surfaces the read error.
	if err := confirmDestructive(confirmTier2, "TARGET", "wipe it", false, false, c7ttyFile(t, ""), &out); err == nil {
		t.Error("tier2 empty input should error")
	}

	// tier1: "yes" confirms.
	if err := confirmDestructive(confirmTier1, "", "wipe it", false, false, c7ttyFile(t, "yes\n"), &out); err != nil {
		t.Errorf("tier1 match = %v", err)
	}
	// tier1: anything else aborts.
	if err := confirmDestructive(confirmTier1, "", "wipe it", false, false, c7ttyFile(t, "no\n"), &out); err != errAborted {
		t.Errorf("tier1 mismatch = %v, want errAborted", err)
	}
	// tier1: no input surfaces the read error.
	if err := confirmDestructive(confirmTier1, "", "wipe it", false, false, c7ttyFile(t, ""), &out); err == nil {
		t.Error("tier1 empty input should error")
	}
}
