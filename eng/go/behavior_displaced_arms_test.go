package eng

import "testing"

// TestDisplacedBehaviorMatchArms keeps the delegate one-liners covered
// after the `is` one-predicate shortcut re-routed the paths that used to
// exercise them: errorConvertBehavior's Match/Equal and
// memberBehavior.Equal are plain kernel-default delegates.
func TestDisplacedBehaviorMatchArms(t *testing.T) {
	ev := NewError(&AqlError{Code: "x", Detail: "m"})
	if !(errorConvertBehavior{}).Match(ev, TError) {
		t.Fatal("an Error value matches TError via the default delegate")
	}
	if !(errorConvertBehavior{}).Equal(ev, ev) || (errorConvertBehavior{}).Equal(ev, NewInteger(1)) {
		t.Fatal("errorConvertBehavior.Equal delegates to the kernel default")
	}
	mb := MemberBehavior(func(v Value) bool { return true }).(memberBehavior)
	if !mb.Equal(NewInteger(1), NewInteger(1)) || mb.Equal(NewInteger(1), NewInteger(2)) {
		t.Fatal("memberBehavior.Equal delegates to the kernel default")
	}
}
