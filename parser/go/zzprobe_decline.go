package parser

// TEMPORARY PROBE FILE — delete before finishing.
// A verbatim copy of grammar.go's matchBasePrefixRun +
// setupDecimalBoundaryMatcher with the dataMode "claim the whole prose run"
// arm DELETED, so data mode simply DECLINES a dot-adjacent base-prefixed run
// to the stock scanners. Used to measure what SafeParseData would return
// after the candidate simplification.

import (
	"strconv"
	"strings"

	jsonic "github.com/tabnas/jsonic/go"
)

// probeSafeParseData mirrors SafeParseData exactly, but installs the
// decline-only data matcher above.
func probeSafeParseData(src string) (any, error) {
	jsonicMakeMu.Lock()
	j := jsonic.Make()
	jsonicMakeMu.Unlock()
	probeSetupDecimalBoundaryMatcher(j, true)
	setupNumberSub(j)
	return j.Parse(src)
}

// matchBasePrefixRun is the 0x/0o/0b arm of the decimal boundary matcher:
// claim base-prefixed integers even when their magnitude exceeds int64
// (Go's stock lexer otherwise drops them to #TX while TS keeps #NR, making
// the public LexTokens streams disagree; the converter reads the exact
// source, so the placeholder value is intentionally irrelevant). The token
// starts at start (which may include a sign) with the `0` at si. A nil
// return declines to the stock scanners.
//
// Data mode cannot decline a dot-adjacent base-prefixed run to the stock
// scanners: they DISAGREE there (Go keeps `0xFF.5` one lenient text, TS
// lexes the prefix as a number and splits the rest into stray values).
// Claim the whole prose run to the data boundary as one text token
// instead — the same string Go's stock scanner produces, now guaranteed
// in both ports. An empty body (`0x.5`, `0xzz`) still declines: the stock
// scanners agree on those.
func probeMatchBasePrefixRun(lex *jsonic.Lex, s string, start, si int, dataMode bool,
	isFollowingText func(string, int) bool) *jsonic.Token {
	cursor := lex.Cursor()
	prefix := s[si+1]
	end := si + 2
	bodyStart := end
	for end < len(s) && (isBasePrefixDigit(s[end], prefix) || s[end] == '_') {
		end++
	}
	if end == bodyStart || isFollowingText(s, end) {
		return nil
	}
	src := s[start:end]
	tkn := lex.Token("#NR", jsonic.TinNR, float64(0), src)
	cursor.SI = end
	cursor.CI += end - start
	return tkn
}

// setupDecimalBoundaryMatcher registers the shared matcher body. dataMode
// selects the boundary alphabet and the no-split rule described on the two
// wrappers above; the language mode is bit-for-bit the historical behavior.
func probeSetupDecimalBoundaryMatcher(j *jsonic.Jsonic, dataMode bool) {
	isDigit := func(c byte) bool { return c >= '0' && c <= '9' }
	isDigitOrSep := func(c byte) bool { return isDigit(c) || c == '_' }
	isFollowingText := func(s string, i int) bool {
		if i >= len(s) {
			return false
		}
		switch s[i] {
		case ' ', '\t', '\n', '\r', '\'', '"', '#', ',', ':', '[', ']', '{', '}', '`':
			return false
		case ';', '(', ')', '?', '.', '!', '|', '<', '>':
			// Language operators end a token only in language mode; a plain
			// data grammar lexes them as ordinary text continuation.
			return dataMode
		case '=':
			if dataMode {
				return true
			}
			return i+1 >= len(s) || s[i+1] != '>'
		default:
			return true
		}
	}

	addMatcher(j, "probe_decimal", 1000004, func(lex *jsonic.Lex, _ *jsonic.Rule) *jsonic.Token {
		cursor := lex.Cursor()
		s := lex.Src
		start := cursor.SI
		si := start
		if si >= len(s) {
			return nil
		}

		hasSign := s[si] == '+' || s[si] == '-'
		if hasSign {
			si++
			if si >= len(s) {
				return nil
			}
		}

		leadingDot := s[si] == '.'
		integerEnd := -1
		if leadingDot {
			// Keep the bare `.5` path on the fixed-dot matcher so it retains
			// the receiverless-dot diagnostic. Signed forms are one numeric
			// token and are rejected later with their complete source.
			if !hasSign || si+1 >= len(s) || !isDigit(s[si+1]) {
				return nil
			}
			si++
			for si < len(s) && isDigitOrSep(s[si]) {
				si++
			}
		} else {
			if !isDigit(s[si]) {
				return nil
			}
			// Big numbers keep their dedicated matcher (which has higher
			// priority); decline here as well so this matcher is safe alone.
			if s[si] == '0' && si+1 < len(s) && strings.ContainsRune("dD", rune(s[si+1])) {
				return nil
			}
			if s[si] == '0' && si+1 < len(s) && strings.ContainsRune("xXoObB", rune(s[si+1])) {
				return probeMatchBasePrefixRun(lex, s, start, si, dataMode, isFollowingText)
			}
			for si < len(s) && isDigitOrSep(s[si]) {
				si++
			}
			integerEnd = si
			if si < len(s) && s[si] == '.' {
				si++
				for si < len(s) && isDigitOrSep(s[si]) {
					si++
				}
			}
		}

		hasDot := leadingDot || integerEnd >= 0 && integerEnd < si && s[integerEnd] == '.'
		if si < len(s) && (s[si] == 'e' || s[si] == 'E') {
			exponentMark := si
			si++
			if si < len(s) && (s[si] == '+' || s[si] == '-') {
				si++
			}
			exponentStart := si
			for si < len(s) && isDigitOrSep(s[si]) {
				si++
			}
			if si == exponentStart {
				if integerEnd >= 0 && hasDot {
					if dataMode {
						return nil
					}
					return emitDecimalPrefix(lex, s, start, integerEnd)
				}
				if leadingDot {
					si = exponentMark
				} else {
					return nil
				}
			}
		}

		if isFollowingText(s, si) {
			// Data mode never splits a run: declining hands the whole span
			// to the stock lenient text scanner, so digit-led prose stays
			// one string instead of shedding a stray second value.
			if dataMode {
				return nil
			}
			if integerEnd >= 0 && hasDot {
				return emitDecimalPrefix(lex, s, start, integerEnd)
			}
			if !leadingDot {
				return nil
			}
		}

		src := s[start:si]
		val, err := strconv.ParseFloat(stripUnderscores(src), 64)
		if err != nil && !isRangeError(err) {
			// A loose separator run such as `1e_` strips to an invalid
			// float, but the converter will reject `_` before reading Val.
			val = 0
		}
		tkn := lex.Token("#NR", jsonic.TinNR, val, src)
		cursor.SI = si
		cursor.CI += si - start
		return tkn
	})
}
