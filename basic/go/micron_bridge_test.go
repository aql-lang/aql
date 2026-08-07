package basic

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestMicronRenderBridgeBackstop pins the display-string backstop in
// eng (value.go, kernelFormatDefault): a Micron instance rendered
// through the KERNEL DEFAULT — the path a wrapper Behavior that
// delegates Format to the default takes — reaches the bridge this
// package installs at init, and the bridge output IS micronRender's.
func TestMicronRenderBridgeBackstop(t *testing.T) {
	r := newTestRegistry(t)
	kind := r.Types.MintType("Bridgeon", TMicron)
	om := NewOrderedMap()
	om.Set("user", NewString("u"))
	om.Set("host", NewString("h.io"))
	v := core.NewValueRaw(kind, MicronPayload{Fields: om})

	got := core.DefaultBehavior.Format(v)
	if got != micronRender(v) {
		t.Errorf("backstop render = %q, want micronRender's %q", got, micronRender(v))
	}
	if !strings.Contains(got, "u") || !strings.Contains(got, "h.io") {
		t.Errorf("backstop render lost the fields: %q", got)
	}
}
