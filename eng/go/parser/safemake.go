package parser

import (
	"sync"

	jsonic "github.com/tabnas/jsonic/go"
)

// jsonicMakeMu serializes every jsonic parser construction in the process.
//
// tabnas/parser's Make bumps a package-global instance-id counter
// (options.go: idCounter++) with no synchronization, so two goroutines
// constructing parsers at once are a data race — caught by `go test -race`
// in TestSpecCompiledConcurrentRowsRaceFree once boru migrated from
// jsonicjs to tabnas/jsonic. Construction is cheap and a constructed
// instance parses independently, so serializing only the Make call removes
// the race while leaving the (expensive) Parse step fully concurrent.
//
// Every production jsonic construction routes through SafeMake / SafeParse /
// GuardMake so they all share this one lock. Do NOT call jsonic.Make /
// jsonic.Parse / multisource.MakeJsonic directly in non-test code.
var jsonicMakeMu sync.Mutex

// SafeMake constructs a jsonic parser under the shared construction lock.
func SafeMake(opts ...jsonic.Options) *jsonic.Jsonic {
	jsonicMakeMu.Lock()
	defer jsonicMakeMu.Unlock()
	return jsonic.Make(opts...)
}

// SafeParse parses src with a freshly-constructed default parser under the
// shared construction lock, then releases it before the parse runs so
// parsing stays concurrent. (jsonic.Parse memoizes a singleton via
// sync.Once whose first call still bumps the shared counter, so it cannot
// be left to race with SafeMake — building our own instance keeps every
// counter access under the lock.)
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
// numbers via ConvertParsedNumber. Construction is serialized under the
// shared lock; the parse itself runs concurrently, as in SafeParse.
func SafeParseData(src string) (any, error) {
	jsonicMakeMu.Lock()
	j := jsonic.Make()
	jsonicMakeMu.Unlock()
	setupNumberSub(j)
	return j.Parse(src)
}

// GuardMake runs an arbitrary jsonic-parser constructor (e.g.
// multisource.MakeJsonic, which the parser package cannot import directly)
// under the shared construction lock and returns its result.
func GuardMake[T any](construct func() T) T {
	jsonicMakeMu.Lock()
	defer jsonicMakeMu.Unlock()
	return construct()
}
