package help

import (
	"errors"
	"regexp"
	"strings"

	core "github.com/boru-lang/boru/core/go"
)

// NoValueMarker is what an example renders when it leaves the stack
// empty — `2 drop ;# (no value)`. It is a documented render, not a
// placeholder: verifiers map it back to the empty stack when re-running.
const NoValueMarker = "(no value)"

// ErrorPrefix opens the render for an example the engine refuses:
// `add true false ;# error [boru/type_error]`.
const ErrorPrefix = "error [boru/"

// identityRender matches the run-specific identity boru embeds in a class
// type name (`T_4f5a79d3674a`) or a process id (`P_fd4ec55d00a0`). Both
// are minted per run, so a result containing one is not a stable value:
// baking it ships a figure that is wrong for every reader and makes the
// generator irreproducible.
var identityRender = regexp.MustCompile(`\b[TP]_[0-9a-f]{12}\b`)

// EncodeExampleResult turns one example evaluation into the string
// `describe` should render, reporting false when there is nothing
// documentable.
//
// It is the SINGLE encoding for both paths that compute example results —
// cmd/go/genhelp at build time and GenerateDynamicExamples at run time —
// so a word registered after MarkReady renders the same forms as a
// built-in one.
//
// Not documentable, and why:
//   - an uncoded failure — a generator bug, not documentation;
//   - `undefined_word` — an artefact of the evaluating registry, not of
//     the example. A module word (`upper`, `xnor`, …) is undefined only
//     because that registry has not imported its module; recording it
//     would assert the word does not exist;
//   - an identity-bearing render (see identityRender).
func EncodeExampleResult(result string, err error) (string, bool) {
	if err != nil {
		var be *core.BoruError
		if !errors.As(err, &be) || be.Code == "" || be.Code == "undefined_word" {
			return "", false
		}
		return ErrorPrefix + be.Code + "]", true
	}
	if result == "" {
		// Only an empty STACK renders "": an empty string VALUE renders
		// as '' (quoted), so this cannot collide.
		return NoValueMarker, true
	}
	if identityRender.MatchString(result) {
		return "", false
	}
	return result, true
}

// IsRefusalRender reports whether an encoded result documents a refusal.
// Such a render is the one kind a LATER definition change can falsify —
// an embedder registering a Boolean `add` overload makes
// `add true false ;# error [boru/type_error]` wrong — so the dynamic
// path re-evaluates these rather than trusting the build-time snapshot.
func IsRefusalRender(encoded string) bool {
	return strings.HasPrefix(encoded, ErrorPrefix)
}
