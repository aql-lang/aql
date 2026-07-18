//go:build linux || darwin

package capabilities

import (
	"errors"
	"strings"
	"testing"
)

// xattr_unix_test.go drives every arm of the OS xattr quartet through
// the syscall seams — real-filesystem xattr support varies by runner
// mount, so no arm's coverage may depend on the host.

func swapXattrSeams(t *testing.T) {
	t.Helper()
	g, s, l, r := unixGetxattr, unixSetxattr, unixListxattr, unixRemovexattr
	t.Cleanup(func() { unixGetxattr, unixSetxattr, unixListxattr, unixRemovexattr = g, s, l, r })
}

func TestOSXattrGetArms(t *testing.T) {
	swapXattrSeams(t)
	boom := errors.New("probe boom")

	// Probe error.
	unixGetxattr = func(string, string, []byte) (int, error) { return 0, boom }
	if _, err := osXattrGet("/p", "a"); !errors.Is(err, boom) {
		t.Errorf("probe error arm: %v", err)
	}
	// Empty value short-circuits without a second call.
	unixGetxattr = func(_, _ string, dest []byte) (int, error) {
		if dest != nil {
			t.Fatal("empty-value get should not fetch")
		}
		return 0, nil
	}
	if v, err := osXattrGet("/p", "a"); err != nil || len(v) != 0 {
		t.Errorf("empty-value arm = %v (%v)", v, err)
	}
	// Fetch error after a successful probe.
	unixGetxattr = func(_, _ string, dest []byte) (int, error) {
		if dest == nil {
			return 3, nil
		}
		return 0, boom
	}
	if _, err := osXattrGet("/p", "a"); !errors.Is(err, boom) {
		t.Errorf("fetch error arm: %v", err)
	}
	// Happy path (value shorter than the probe said — n wins).
	unixGetxattr = func(_, _ string, dest []byte) (int, error) {
		if dest == nil {
			return 3, nil
		}
		copy(dest, "hi!")
		return 2, nil
	}
	if v, err := osXattrGet("/p", "a"); err != nil || string(v) != "hi" {
		t.Errorf("happy arm = %q (%v)", v, err)
	}
}

func TestOSXattrSetRemoveArms(t *testing.T) {
	swapXattrSeams(t)
	boom := errors.New("set boom")
	unixSetxattr = func(string, string, []byte, int) error { return boom }
	if err := osXattrSet("/p", "a", nil); !errors.Is(err, boom) {
		t.Errorf("set error arm: %v", err)
	}
	unixSetxattr = func(string, string, []byte, int) error { return nil }
	if err := osXattrSet("/p", "a", []byte("v")); err != nil {
		t.Errorf("set happy arm: %v", err)
	}
	unixRemovexattr = func(string, string) error { return boom }
	if err := osXattrRemove("/p", "a"); !errors.Is(err, boom) {
		t.Errorf("remove error arm: %v", err)
	}
	unixRemovexattr = func(string, string) error { return nil }
	if err := osXattrRemove("/p", "a"); err != nil {
		t.Errorf("remove happy arm: %v", err)
	}
}

func TestOSXattrListArms(t *testing.T) {
	swapXattrSeams(t)
	boom := errors.New("list boom")

	unixListxattr = func(string, []byte) (int, error) { return 0, boom }
	if _, err := osXattrList("/p"); !errors.Is(err, boom) {
		t.Errorf("probe error arm: %v", err)
	}
	unixListxattr = func(string, []byte) (int, error) { return 0, nil }
	if names, err := osXattrList("/p"); err != nil || len(names) != 0 {
		t.Errorf("empty arm = %v (%v)", names, err)
	}
	unixListxattr = func(_ string, dest []byte) (int, error) {
		if dest == nil {
			return 4, nil
		}
		return 0, boom
	}
	if _, err := osXattrList("/p"); !errors.Is(err, boom) {
		t.Errorf("fetch error arm: %v", err)
	}
	// Happy path: NUL-separated names arrive UNSORTED (and with a
	// trailing empty segment) and come back sorted.
	payload := []byte("user.b\x00user.a\x00")
	unixListxattr = func(_ string, dest []byte) (int, error) {
		if dest == nil {
			return len(payload), nil
		}
		copy(dest, payload)
		return len(payload), nil
	}
	names, err := osXattrList("/p")
	if err != nil || strings.Join(names, ",") != "user.a,user.b" {
		t.Errorf("happy arm = %v (%v)", names, err)
	}
}
