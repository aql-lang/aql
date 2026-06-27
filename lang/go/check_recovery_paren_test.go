package lang

import (
	"strings"
	"testing"
)

// TestRecoveryNoPhantomParen pins the recovery forward-collection paren fix
// (engine.go checkModeFallbackPositions). When a dispatch recovers (an Any-typed
// operand) inside a paren group and its assumed arity EXCEEDS the args present
// before the group's `)`, the recovery used to gather candidate positions PAST the
// close-paren; splicing those out then deleted tokens across the `)` boundary and
// left a phantom "unmatched opening parenthesis" — the emergent whole-module paren
// bleed (template.aql's first-word/after-word/parts fn_body_errors). The fix tracks
// forward-group depth and stops at the enclosing `)`.
//
// `need2` (2-arg) is dispatched over `((d get 0) size)` (one group arg, an Any-typed
// element from `get`) — assumed arity 2 but only one arg before the outer `)`, so the
// recovery over-reaches. The program must now check WITHOUT any unmatched-paren error
// (other diagnostics like a no_signature over the gradual arg are unrelated).
func TestRecoveryNoPhantomParen(t *testing.T) {
	const src = `import "aql:string-util"
def need2 fn [ [a:String b:String] [Integer] [ 1 ] ]
def after fn [ [s:String] [String] [ (StringUtil.trim (slice 0 1 s)) ] ]
def d [ {x:1} ]
def _ (need2 ((d get 0) size))
("hi" after) print`

	a, _ := New()
	res, _ := a.Check(src)
	for _, diag := range res.Diagnostics {
		blob := diag.Code + " " + diag.Detail
		if strings.Contains(blob, "unmatched") || strings.Contains(blob, "parenthesis") {
			t.Errorf("phantom paren error from recovery over-gather: %s: %s", diag.Code, diag.Detail)
		}
	}
}
