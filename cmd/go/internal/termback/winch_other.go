//go:build !unix

package termback

// platformResizeSignal has no size-change source off unix (Windows VT
// input records are future work — TUI.0.md §9 best-effort note).
func platformResizeSignal(stop <-chan struct{}) <-chan struct{} {
	_ = stop
	return nil
}
