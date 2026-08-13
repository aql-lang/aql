package modules

import (
	"github.com/boru-lang/boru/lang/go/native"
)

// Check-mode mirrors for the boru:tui words that validate their option
// and app maps BEFORE touching the terminal — the headless contract
// lang/spec/module-tui.tsv §§3-6 pins, and the same shape the boru:vault
// usage mirrors follow (vault.go). Each mirror runs the handler's OWN
// pure prefix over deep-concrete operands, so the diagnostic carries the
// byte-identical code and detail the run raises.
//
// Why these words and not the rest of the surface: a Tier-1 word takes a
// Terminal handle, which only `open` can mint, so its refusals are
// backend-state, not argument shape. `open` / `run` / `serve` take maps
// the call site writes literally, and every check below is a pure
// function of those maps.

// tuiUsageMirror wires one word's pure prefix into a gated mirror.
//
// The policy gate is consulted but never reported: when the policy denies
// the word the run raises the refusal and never reaches the validation,
// so claiming a usage defect there would name the wrong error. Declining
// keeps the mirror firing exactly where the run does.
func tuiUsageMirror(word, polOp string, arity int,
	validate func([]native.Value, *native.Registry) error, base native.ReturnsFunc) native.ReturnsFunc {
	return native.MirrorReturns(word, concreteArgsGate(arity),
		func(args []native.Value, r *native.Registry) error {
			if polErr := checkTuiPolicy(r, word, polOp); polErr != nil {
				return nil
			}
			return validate(args, r)
		}, base)
}

// tuiDynAnyReturns is the residual `run` / `serve` declare — a DYNAMIC Any
// carrier, exactly what declaredReturnCarriers builds for a declared Any
// (a strict Any conforms to no typed slot and would poison every consumer
// downstream). Attaching a ReturnsFn replaces Returns, so the mirror has
// to reproduce it.
func tuiDynAnyReturns(_ []native.Value, _ *native.Registry) []native.Value {
	c := native.NewCarrier(native.TAny)
	c.Dynamic = true
	return []native.Value{c}
}

// tuiOpenMirror models tuiOpenHandler's option parse: the mouse / title
// readers and the §11.7 alt-screen reservation. It stops where the
// handler stops being pure — acquireTuiBackend, which needs a host.
func tuiOpenMirror() native.ReturnsFunc {
	return tuiUsageMirror("open", "open", 1,
		func(args []native.Value, r *native.Registry) error {
			mp, _ := native.AsMap(args[0])
			if mp == nil {
				return nil // not an options map — dispatch owns that refusal
			}
			var err error
			if _, err = tuiOptBoolean(mp, "mouse"); err == nil {
				_, err = tuiOptString(mp, "title")
			}
			if err != nil {
				return r.BoruError("tui_error", "open: "+err.Error(), "open")
			}
			return tuiAltScreenReserved(mp, "open", r)
		},
		func(_ []native.Value, _ *native.Registry) []native.Value {
			return []native.Value{native.NewCarrier(TTerminal)}
		})
}

// tuiRunMirror models tuiRunHandler's app-config parse — the whole of
// parseTuiApp, which is the handler's entire prefix before the process
// runtime is touched.
func tuiRunMirror() native.ReturnsFunc {
	return tuiUsageMirror("run", "open", 1,
		func(args []native.Value, r *native.Registry) error {
			_, err := parseTuiApp(args[0], "run", r)
			return err
		}, tuiDynAnyReturns)
}

// tuiServeMirror models tuiServeHandler's prefix in the handler's order:
// the transport options first, then the app config. The net policy gate
// between them is consulted for the same reason tuiUsageMirror consults
// the tui one — a refusal there means the run never reaches parseTuiApp.
func tuiServeMirror() native.ReturnsFunc {
	return tuiUsageMirror("serve", "serve", 2,
		func(args []native.Value, r *native.Registry) error {
			opts, err := tuiServeOptsOf(args[0], r)
			if err != nil {
				return err
			}
			if netErr := checkNetPolicy(r, "serve", "listen", "", opts.port); netErr != nil {
				return nil
			}
			_, err = parseTuiApp(args[1], "serve", r)
			return err
		}, tuiDynAnyReturns)
}
