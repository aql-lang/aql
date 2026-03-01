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
