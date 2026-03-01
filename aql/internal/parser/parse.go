package parser

import (
	"fmt"
	"strconv"
	"strings"

	jsonic "github.com/jsonicjs/jsonic/go"
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// Custom Tin values for AQL-specific fixed tokens.
var (
	tinOP = jsonic.TinMAX     // Open parenthesis (
	tinCP = jsonic.TinMAX + 1 // Close parenthesis )
)

// newConfig returns a jsonic LexConfig customized for AQL tokenization.
func newConfig() *jsonic.LexConfig {
	cfg := jsonic.DefaultLexConfig()

	// Register ( and ) as fixed tokens so they delimit adjacent text.
	cfg.FixedTokens["("] = tinOP
	cfg.FixedTokens[")"] = tinCP
	cfg.TinNames = map[jsonic.Tin]string{
		tinOP: "#OP",
		tinCP: "#CP",
	}
	cfg.SortFixedTokens()

	// Disable value-keyword recognition so true, false, null become plain words.
	cfg.ValueLex = false

	return cfg
}

// Parse tokenizes the AQL source string into a slice of engine.Value.
// The input is treated as an implicit list: each token in the source becomes
// one element in the returned slice.
func Parse(src string) ([]engine.Value, error) {
	cfg := newConfig()
	lex := jsonic.NewLex(src, cfg)

	values, err := parseTokens(lex, jsonic.TinZZ)
	if err != nil {
		return nil, err
	}

	if lex.Err != nil {
		return nil, fmt.Errorf("parse error: %v", lex.Err)
	}

	return values, nil
}

// parseTokens reads tokens from the lexer until it encounters stopTin.
// It handles recursive map ({...}) and list ([...]) literals.
func parseTokens(lex *jsonic.Lex, stopTin jsonic.Tin) ([]engine.Value, error) {
	var values []engine.Value

	for {
		tok := lex.Next()
		if tok.Tin == stopTin {
			break
		}
		if tok.Tin == jsonic.TinZZ {
			if stopTin != jsonic.TinZZ {
				return nil, fmt.Errorf("parse error: unexpected end of input")
			}
			break
		}

		switch tok.Tin {
		case jsonic.TinNR:
			n, ok := tok.Val.(float64)
			if !ok {
				return nil, fmt.Errorf("parse error at %d:%d: expected number", tok.RI, tok.CI)
			}
			values = append(values, engine.NewInteger(int64(n)))

		case jsonic.TinST:
			s, ok := tok.Val.(string)
			if !ok {
				return nil, fmt.Errorf("parse error at %d:%d: expected string", tok.RI, tok.CI)
			}
			values = append(values, engine.NewString(s))

		case jsonic.TinTX:
			v, err := parseWord(tok.Src)
			if err != nil {
				return nil, fmt.Errorf("parse error at %d:%d: %w", tok.RI, tok.CI, err)
			}
			values = append(values, v)

		case tinOP:
			values = append(values, engine.NewWord("("))

		case tinCP:
			values = append(values, engine.NewWord(")"))

		case jsonic.TinOS:
			// List literal: [elem, elem, ...]
			listVal, err := parseList(lex)
			if err != nil {
				return nil, err
			}
			values = append(values, listVal)

		case jsonic.TinOB:
			// Map literal: {key:val, key:val, ...}
			mapVal, err := parseMap(lex)
			if err != nil {
				return nil, err
			}
			values = append(values, mapVal)

		case jsonic.TinCA:
			// Commas are separators inside lists/maps; at top level, skip.
			continue

		default:
			return nil, fmt.Errorf("parse error at %d:%d: unexpected token %s %q",
				tok.RI, tok.CI, tok.Name, tok.Src)
		}
	}

	return values, nil
}

// parseList reads list elements until ] is encountered.
// Elements are separated by commas or whitespace.
func parseList(lex *jsonic.Lex) (engine.Value, error) {
	var elems []engine.Value

	for {
		tok := lex.Next()
		if tok.Tin == jsonic.TinZZ {
			return engine.Value{}, fmt.Errorf("parse error: unterminated list literal")
		}
		if tok.Tin == jsonic.TinCS {
			break
		}
		if tok.Tin == jsonic.TinCA {
			continue // skip commas
		}

		val, err := parseValueToken(tok, lex)
		if err != nil {
			return engine.Value{}, err
		}
		elems = append(elems, val)
	}

	return engine.NewList(elems), nil
}

// parseMap reads key:value pairs until } is encountered.
// Pairs are separated by commas or whitespace.
func parseMap(lex *jsonic.Lex) (engine.Value, error) {
	entries := engine.NewOrderedMap()

	for {
		// Read key.
		keyTok := lex.Next()
		if keyTok.Tin == jsonic.TinZZ {
			return engine.Value{}, fmt.Errorf("parse error: unterminated map literal")
		}
		if keyTok.Tin == jsonic.TinCB {
			break
		}
		if keyTok.Tin == jsonic.TinCA {
			continue // skip commas
		}

		var key string
		switch keyTok.Tin {
		case jsonic.TinTX:
			key = keyTok.Src
		case jsonic.TinST:
			s, ok := keyTok.Val.(string)
			if !ok {
				return engine.Value{}, fmt.Errorf("parse error at %d:%d: expected string key", keyTok.RI, keyTok.CI)
			}
			key = s
		default:
			return engine.Value{}, fmt.Errorf("parse error at %d:%d: expected map key, got %s", keyTok.RI, keyTok.CI, keyTok.Name)
		}

		// Expect colon.
		colonTok := lex.Next()
		if colonTok.Tin != jsonic.TinCL {
			return engine.Value{}, fmt.Errorf("parse error at %d:%d: expected ':' after map key %q", colonTok.RI, colonTok.CI, key)
		}

		// Read value.
		valTok := lex.Next()
		if valTok.Tin == jsonic.TinZZ {
			return engine.Value{}, fmt.Errorf("parse error: unterminated map literal")
		}

		val, err := parseValueToken(valTok, lex)
		if err != nil {
			return engine.Value{}, err
		}

		entries.Set(key, val)
	}

	return engine.NewMap(entries), nil
}

// parseValueToken converts a single token into an engine.Value.
// It handles recursive structures (nested maps and lists).
func parseValueToken(tok *jsonic.Token, lex *jsonic.Lex) (engine.Value, error) {
	switch tok.Tin {
	case jsonic.TinNR:
		n, ok := tok.Val.(float64)
		if !ok {
			return engine.Value{}, fmt.Errorf("parse error at %d:%d: expected number", tok.RI, tok.CI)
		}
		return engine.NewInteger(int64(n)), nil

	case jsonic.TinST:
		s, ok := tok.Val.(string)
		if !ok {
			return engine.Value{}, fmt.Errorf("parse error at %d:%d: expected string", tok.RI, tok.CI)
		}
		return engine.NewString(s), nil

	case jsonic.TinTX:
		return parseValueWord(tok.Src), nil

	case jsonic.TinOS:
		return parseList(lex)

	case jsonic.TinOB:
		return parseMap(lex)

	default:
		return engine.Value{}, fmt.Errorf("parse error at %d:%d: unexpected token %s in structure", tok.RI, tok.CI, tok.Name)
	}
}

// parseValueWord converts a bare word inside a map/list to the appropriate value.
// Type names and boolean literals are resolved; other words become strings.
func parseValueWord(text string) engine.Value {
	// Boolean literals.
	if text == "true" {
		return engine.NewBoolean(true)
	}
	if text == "false" {
		return engine.NewBoolean(false)
	}

	// Well-known type names resolve to type literals.
	typeNames := map[string]engine.Type{
		"any":     engine.TAny,
		"none":    engine.TNone,
		"number":  engine.TNumber,
		"string":  engine.TString,
		"boolean": engine.TBoolean,
		"list":    engine.TList,
		"map":     engine.TMap,
	}
	if t, ok := typeNames[text]; ok {
		return engine.NewTypeLiteral(t)
	}

	// Unknown words become strings (matching AQL engine behavior).
	return engine.NewString(text)
}

// parseWord interprets an unquoted text token as an AQL word, handling
// modifier syntax: name/s (forceSuffix), name/p (forcePrefix), name/N (argCount),
// and combinations like name/1s or name/2p.
func parseWord(text string) (engine.Value, error) {
	name := text
	argCount := -1
	forcePrefix := false
	forceSuffix := false

	// Check for /... modifier suffix.
	if idx := strings.LastIndex(name, "/"); idx >= 0 && idx < len(name)-1 {
		mod := name[idx+1:]
		name = name[:idx]

		// Parse optional digits followed by optional 's' or 'p'.
		digits := ""
		rest := mod
		for i, c := range rest {
			if c >= '0' && c <= '9' {
				digits += string(c)
			} else {
				rest = rest[i:]
				break
			}
			if i == len(rest)-1 {
				rest = ""
			}
		}

		if digits != "" {
			n, err := strconv.Atoi(digits)
			if err == nil && n >= 0 {
				argCount = n
			}
		}

		switch rest {
		case "s":
			forceSuffix = true
		case "p":
			forcePrefix = true
		case "":
			// digits only, no mode flag
			if digits == "" {
				// No digits and no flag — not a valid modifier; restore name
				name = text
			}
		default:
			// Unrecognized modifier — treat entire token as plain word
			name = text
		}
	}

	if name == "" {
		return engine.Value{}, fmt.Errorf("empty word")
	}

	if forcePrefix || forceSuffix || argCount >= 0 {
		return engine.NewWordModified(name, argCount, forcePrefix, forceSuffix), nil
	}
	return engine.NewWord(name), nil
}
