package parser

import (
	jsonic "github.com/tabnas/jsonic/go"
)

// SafeMake constructs a jsonic parser.
//
// It is the single construction seam for the whole repo — the Go twin of
// parser/ts's safeMake, which additionally normalizes string enders — so
// construction policy has one place to live and both ports stay symmetric.
//
// HISTORY. Until tabnas/parser v0.8.0 this function (and SafeParse,
// SafeParseData and a GuardMake wrapper) serialized every construction
// behind a process-wide mutex, because Make bumped a package-global
// instance-id counter with no synchronization — a data race `go test -race`
// caught in TestSpecCompiledConcurrentRowsRaceFree. v0.8.0 made that
// counter atomic and documents Make as safe for concurrent use, so the lock
// is gone (ADR-014: the fix belongs upstream, and the shim goes when it
// lands). GuardMake went with it: its whole signature existed to run a
// caller's constructor under that lock.
func SafeMake(opts ...jsonic.Options) *jsonic.Jsonic {
	return jsonic.Make(opts...)
}

// SafeParse parses src with a freshly-constructed default parser. Its errors
// deliberately remain native jsonic errors; ParseConfig owns the stable
// host-facing wrapper for its narrower configuration contract.
func SafeParse(src string) (any, error) {
	return SafeMake().Parse(src)
}

// SafeParseData parses src like SafeParse but ATTACHES the number-Sub
// callback (setupNumberSub), so numeric tokens come back wrapped in a
// numberVal carrying their exact source text. This preserves the
// integer/float distinction (and exact large-integer precision) the way the
// boru-source parser does — "42.0" stays a Float, "42" an Integer, an
// out-of-range integer raises [boru/integer_overflow] — instead of the bare
// float64 collapse a default parser produces (where "42.0" silently becomes
// Integer 42). Use this for data-decode paths that must round-trip numeric
// types faithfully, notably StructUtil.parse. Callers convert the wrapped
// numbers via ConvertParsedNumber.
func SafeParseData(src string) (any, error) {
	j := jsonic.Make()
	// Keep every COMPLETE numeric spelling that Boru classifies itself on
	// one #NR token. The stock Go jsonic matcher drops trailing-dot
	// exponents, binary64 overflow, and base-prefix overflow to plain
	// values/text, which would make ConvertParsedNumber silently decline
	// them. The data-mode matcher claims only complete tokens and never
	// splits: digit-led prose (`1.x`, `1.2.3`, `1.2-beta`) stays on the
	// stock lenient text path, preserving the jsonic-superset contract.
	setupDataDecimalMatcher(j)
	setupNumberSub(j)
	return j.Parse(src)
}
