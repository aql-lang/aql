package native

import (
	"io"
	"os"
)

// resolve_color.go — the colour decision for diagnostic output.
//
// This lived in eng beside the renderer that consumes its verdict, which was
// adjacency rather than need: eng never called it, every caller is in cmd/go,
// and it was the one place in the kernel's diagnostic renderer that reached
// for ambient state (the process environment, and a stat() on the
// destination). The kernel is purely algorithmic — it takes the verdict as a
// RenderOpts field and formats text — so the decision belongs here, in the
// layer that already owns host capabilities.
//
// Moving it also closes the divergence that made it worth moving: NO_COLOR
// can now be read through the host's installed environment view, so the
// renderer and `IO.env` agree about what the environment IS.

// ResolveColor decides whether to color output written to w, per the
// conventional cascade: mode "always"/"never" is explicit; anything else (the
// "auto" default) colors only when NO_COLOR is unset and w is a character
// device (a real terminal). Non-file writers (buffers, pipes wrapped in
// custom types) never auto-color.
//
// r supplies the environment NO_COLOR is read from. When the host installed
// an EnvOps, that view governs — a host that filters or hides the variable
// hides it from the renderer too, which is what a capability is for. A nil
// registry, or one with no EnvOps installed, falls back to the process: that
// is the right default for a CLI styling its OWN output before any program
// instance exists.
func ResolveColor(r *Registry, w io.Writer, mode string) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	}
	if noColor(r) {
		return false
	}
	return isCharDevice(w)
}

// noColor reports whether NO_COLOR is set to a non-empty value, through the
// host's environment view when there is one.
func noColor(r *Registry) bool {
	if ops := HostEnvOps(r); ops != nil {
		v, _ := ops.Get("NO_COLOR")
		return v != ""
	}
	return os.Getenv("NO_COLOR") != ""
}

// isCharDevice reports whether w is a terminal, by the same character-device
// test the aql:io `is-tty` word uses (capabilities.OSStreamProbe), so a word
// and the renderer can never disagree about a stream. Engine-side writer
// wrappers are peeled first: every concurrency fork rewraps the output
// writers, and an *os.File assertion through the wrapper would report a real
// terminal as "not a terminal".
func isCharDevice(w io.Writer) bool {
	f, ok := unwrapWriter(w).(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}
