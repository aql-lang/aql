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

// typeNames maps well-known type names to their engine Type.
var typeNames = map[string]engine.Type{
	"any":     engine.TAny,
	"none":    engine.TNone,
	"number":  engine.TNumber,
	"string":  engine.TString,
	"boolean": engine.TBoolean,
	"list":    engine.TList,
	"map":     engine.TMap,
}

// Parse tokenizes the AQL source string into a slice of engine.Value.
// The input is treated as an implicit list: each token in the source becomes
// one element in the returned slice.
func Parse(src string) ([]engine.Value, error) {
	cfg := newConfig()
	lex := jsonic.NewLex(src, cfg)

	var values []engine.Value

	for {
		tok := lex.Next()
		if tok.Tin == jsonic.TinZZ {
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

		case jsonic.TinOB, jsonic.TinOS:
			// Map or list literal — extract the balanced substring from source
			// and delegate to jsonic.Parse, which already handles recursive
			// structure parsing.
			sub, err := extractBracketed(lex.Src, tok.SI)
			if err != nil {
				return nil, fmt.Errorf("parse error at %d:%d: %w", tok.RI, tok.CI, err)
			}
			parsed, err := jsonic.Parse(sub)
			if err != nil {
				return nil, fmt.Errorf("parse error at %d:%d: %w", tok.RI, tok.CI, err)
			}
			val, err := convertGoValue(parsed)
			if err != nil {
				return nil, fmt.Errorf("parse error at %d:%d: %w", tok.RI, tok.CI, err)
			}
			values = append(values, val)

			// Advance the lexer past all the tokens we consumed via jsonic.Parse.
			skipPastBracketed(lex, tok.Tin)

		default:
			return nil, fmt.Errorf("parse error at %d:%d: unexpected token %s %q",
				tok.RI, tok.CI, tok.Name, tok.Src)
		}
	}

	if lex.Err != nil {
		return nil, fmt.Errorf("parse error: %v", lex.Err)
	}

	return values, nil
}

// extractBracketed extracts a balanced bracketed substring from src starting
// at position start. Handles nested brackets and quoted strings.
func extractBracketed(src string, start int) (string, error) {
	open := src[start]
	var close byte
	if open == '{' {
		close = '}'
	} else {
		close = ']'
	}

	depth := 0
	inString := false
	var stringChar byte

	for i := start; i < len(src); i++ {
		c := src[i]

		if inString {
			if c == '\\' {
				i++ // skip escaped char
				continue
			}
			if c == stringChar {
				inString = false
			}
			continue
		}

		if c == '"' || c == '\'' {
			inString = true
			stringChar = c
			continue
		}

		if c == open {
			depth++
		} else if c == close {
			depth--
			if depth == 0 {
				return src[start : i+1], nil
			}
		}
	}

	return "", fmt.Errorf("unterminated %c literal", open)
}

// skipPastBracketed advances the lexer past all tokens belonging to a
// bracketed structure (map or list) that was already parsed by jsonic.Parse.
func skipPastBracketed(lex *jsonic.Lex, openTin jsonic.Tin) {
	var closeTin jsonic.Tin
	if openTin == jsonic.TinOB {
		closeTin = jsonic.TinCB
	} else {
		closeTin = jsonic.TinCS
	}

	depth := 1
	for depth > 0 {
		tok := lex.Next()
		if tok.Tin == jsonic.TinZZ {
			break
		}
		if tok.Tin == openTin {
			depth++
		} else if tok.Tin == closeTin {
			depth--
		}
	}
}

// convertGoValue converts a Go native value (from jsonic.Parse) into an
// engine.Value, recursively handling maps and lists.
func convertGoValue(v any) (engine.Value, error) {
	switch val := v.(type) {
	case map[string]any:
		m := engine.NewOrderedMap()
		for _, key := range sortedKeys(val) {
			child, err := convertGoValue(val[key])
			if err != nil {
				return engine.Value{}, err
			}
			m.Set(key, child)
		}
		return engine.NewMap(m), nil

	case []any:
		elems := make([]engine.Value, len(val))
		for i, elem := range val {
			child, err := convertGoValue(elem)
			if err != nil {
				return engine.Value{}, err
			}
			elems[i] = child
		}
		return engine.NewList(elems), nil

	case float64:
		return engine.NewInteger(int64(val)), nil

	case string:
		return resolveTextValue(val), nil

	case bool:
		return engine.NewBoolean(val), nil

	case nil:
		return engine.NewTypeLiteral(engine.TNone), nil

	default:
		return engine.Value{}, fmt.Errorf("unsupported value type %T", v)
	}
}

// resolveTextValue converts a bare text string (from jsonic) into the
// appropriate AQL value — type literal, boolean, or plain string.
func resolveTextValue(text string) engine.Value {
	if text == "true" {
		return engine.NewBoolean(true)
	}
	if text == "false" {
		return engine.NewBoolean(false)
	}
	if t, ok := typeNames[text]; ok {
		return engine.NewTypeLiteral(t)
	}
	return engine.NewString(text)
}

// sortedKeys returns the keys of a map in sorted order for deterministic output.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Sort for deterministic ordering since Go maps are unordered.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
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
