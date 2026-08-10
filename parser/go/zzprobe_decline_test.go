package parser

// TEMPORARY PROBE — delete before finishing.

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	jsonic "github.com/tabnas/jsonic/go"
)

func probeRenderNode(v any) (string, error) {
	if ev, claimed, cerr := ConvertParsedNumber(v); claimed {
		if cerr != nil {
			return "", cerr
		}
		return core.CanonValue(ev), nil
	}
	switch x := v.(type) {
	case nil:
		return "none", nil
	case bool:
		if x {
			return "true", nil
		}
		return "false", nil
	case string:
		return "'" + x + "'", nil
	case []any:
		items := make([]string, 0, len(x))
		for _, item := range x {
			s, err := probeRenderNode(item)
			if err != nil {
				return "", err
			}
			items = append(items, s)
		}
		return "[" + strings.Join(items, " ") + "]", nil
	case *jsonic.OrderedMap:
		pairs := make([]string, 0, len(x.Keys))
		for _, k := range x.Keys {
			child, _ := x.Get(k)
			s, err := probeRenderNode(child)
			if err != nil {
				return "", err
			}
			pairs = append(pairs, k+":"+s)
		}
		return "{" + strings.Join(pairs, " ") + "}", nil
	}
	return "", fmt.Errorf("unsupported data node %T", v)
}

func probeRender(parse func(string) (any, error), src string) string {
	v, err := parse(src)
	if err != nil {
		return "ERR"
	}
	s, cerr := probeRenderNode(v)
	if cerr != nil {
		var be *core.BoruError
		if !errors.As(cerr, &be) {
			return "HARNESS " + cerr.Error()
		}
		return "ERR " + be.Code
	}
	return s
}

func probeDecodeEscapes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			case 't':
				b.WriteByte('\t')
				i++
				continue
			case '\\':
				b.WriteByte('\\')
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestZZProbeDeclineAllDataRows(t *testing.T) {
	path := filepath.Join("..", "spec", "data.tsv")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	rows := 0
	diffs := 0
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if line == "" || (strings.HasPrefix(line, "#") && !strings.Contains(line, "\t")) {
			continue
		}
		parts := strings.Split(line, "\t")
		src := probeDecodeEscapes(parts[0])
		want := probeDecodeEscapes(parts[1])
		rows++
		cur := probeRender(SafeParseData, src)
		dec := probeRender(probeSafeParseData, src)
		status := "SAME"
		if cur != dec {
			status = "CHANGED"
			diffs++
		}
		okCur := "ok"
		if cur != want {
			okCur = "MISMATCH-CURRENT"
		}
		okDec := "ok"
		if dec != want {
			okDec = "MISMATCH-DECLINE"
		}
		t.Logf("row %2d %-24q want=%-18s current=%-18s decline=%-18s %s cur:%s dec:%s",
			n, src, want, cur, dec, status, okCur, okDec)
	}
	t.Logf("TOTAL rows=%d changed=%d", rows, diffs)
}

// Extra sources beyond the corpus: stress the arm's boundary alphabet.
func TestZZProbeDeclineExtra(t *testing.T) {
	srcs := []string{
		"0xFF.5",
		"[0xFF.5]",
		"{a:0xFF.5}",
		"[-0xFF.5]",
		"[+0xFF.5, 1]",
		"{a: 0xFF.5, b: 2}",
		"0xFF.5.6",
		"[0xFF.5 0x1F.2]",
		"{a: 0x1p3}",
		"0o8.5",
		"0b12.5",
		"[0xFF., 1]",
		"{a: 0xFF.5:2}",
		"[0xFF.5=1]",
		"{a: 0xFF.5;x}",
		"[0xFF.5(1)]",
		"{a: 0xFF.5|x}",
		"{a: 0xFF.5?}",
		"{a: 0xFF.5!}",
		"{a: 0xFF.5<x>}",
		"[0xFF.5,2]",
		"{a: 0xFF.5#c}",
		"[0xFF.5'q']",
		"0xFF.5\n1",
		"[0xFF_.5]",
		"0x_.5",
		"0x_1.5",
		"{a: 0X1F.5}",
		"{a: 0O17.5}",
		"{a: 0B1.5}",
		"0xFF",
		"0x8000000000000000",
		"[0xFF, 1]",
	}
	for _, src := range srcs {
		cur := probeRender(SafeParseData, src)
		dec := probeRender(probeSafeParseData, src)
		bare, berr := SafeParse(src)
		bareStr := "ERR"
		if berr == nil {
			bareStr = fmt.Sprintf("%#v", plainProbe(bare))
		}
		status := "SAME"
		if cur != dec {
			status = "CHANGED"
		}
		t.Logf("%-22q current=%-26s decline=%-26s %-8s bare=%s", src, cur, dec, status, bareStr)
	}
}

func plainProbe(v any) any {
	switch x := v.(type) {
	case []any:
		out := make([]any, 0, len(x))
		for _, it := range x {
			out = append(out, plainProbe(it))
		}
		return out
	case *jsonic.OrderedMap:
		out := map[string]any{}
		for _, k := range x.Keys {
			c, _ := x.Get(k)
			out[k] = plainProbe(c)
		}
		return out
	}
	return v
}

// --- token-level comparison over every spec source, BOTH modes ---

func probeTokens(build func(*jsonic.Jsonic), src string) []string {
	j := SafeMake(jsonic.Options{})
	build(j)
	cfg := j.Config()
	cfg.IgnoreSet = map[jsonic.Tin]bool{}
	lex := jsonic.NewLex(src, cfg)
	var out []string
	for i := 0; i < 100000; i++ {
		tok := lex.Next()
		if tok == nil || tok.Tin == jsonic.TinZZ {
			break
		}
		out = append(out, fmt.Sprintf("%s|%q|%d", tok.Name, tok.Src, tok.SI))
	}
	if lex.Err != nil {
		out = append(out, "LEXERR")
	}
	return out
}

func probeAllSpecSources(t *testing.T) []string {
	t.Helper()
	var srcs []string
	for _, name := range []string{"parse.tsv", "data.tsv", "lex.tsv", "divergent.tsv", "nesting.tsv", "shape.tsv"} {
		b, err := os.ReadFile(filepath.Join("..", "spec", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			if line == "" || (strings.HasPrefix(line, "#") && !strings.Contains(line, "\t")) {
				continue
			}
			srcs = append(srcs, probeDecodeEscapes(strings.Split(line, "\t")[0]))
		}
	}
	return srcs
}

func TestZZProbeTokenStreamsBothModes(t *testing.T) {
	srcs := append(probeAllSpecSources(t), []string{
		"0xFF.5", "[0xFF.5]", "{a:0xFF.5}", "0xFF.5 add 1", "0xFF.foo", "0xFF.5;x",
		"[-0xFF.5, 9]", "0x1_.5", "0xFF.", "0xFFx", "0x.5", "0xzz", "0xFF", "0x8000000000000000",
		"a.0xFF.5", "0o17.e2", "0b101.1", "{a: 0xFF.5=>1}", "0xFF.5=>2", "0xFF.5<x>",
	}...)
	langReal := func(j *jsonic.Jsonic) {
		tk, _ := setupBaseTokens(j, loadDeclGrammar())
		setupTemplateLiteralMatcher(j, tk)
		setupBigNumberMatcher(j, tk)
		setupDecimalUnderscoreMatcher(j, tk)
		setupMiniLitMatcher(j, tk)
	}
	langProbe := func(j *jsonic.Jsonic) {
		tk, _ := setupBaseTokens(j, loadDeclGrammar())
		setupTemplateLiteralMatcher(j, tk)
		setupBigNumberMatcher(j, tk)
		probeSetupDecimalBoundaryMatcher(j, false)
		setupMiniLitMatcher(j, tk)
	}
	dataReal := func(j *jsonic.Jsonic) { setupDataDecimalMatcher(j) }
	dataProbe := func(j *jsonic.Jsonic) { probeSetupDecimalBoundaryMatcher(j, true) }

	langDiff, dataDiff := 0, 0
	for _, src := range srcs {
		a := strings.Join(probeTokens(langReal, src), " ")
		b := strings.Join(probeTokens(langProbe, src), " ")
		if a != b {
			langDiff++
			t.Errorf("LANG token stream differs for %q:\n real: %s\n probe:%s", src, a, b)
		}
		c := strings.Join(probeTokens(dataReal, src), " ")
		d := strings.Join(probeTokens(dataProbe, src), " ")
		if c != d {
			dataDiff++
			t.Logf("DATA token stream differs for %q:\n real: %s\n probe:%s", src, c, d)
		}
	}
	t.Logf("TOTAL sources=%d langDiff=%d dataDiff=%d", len(srcs), langDiff, dataDiff)
}

// Unicode / quote / ender stress: places where the ARM's byte-level
// isFollowingText alphabet might differ from the dependency's ender set.
func TestZZProbeDeclineUnicodeAndEnders(t *testing.T) {
	srcs := []string{
		"{a: 0xFF.5🙂}", "[0xFF.5🙂, 1]", "0xFF.5é", "{a: 0xFF.5\"q\"}", "[0xFF.5`t`]",
		"{a: 0xFF.5\tb}", "[0xFF.5]", "{a: 0xFF.5}", "{a: 0xFF.5", "[0xFF.5",
		"{a: 0xFF.5 }", "[0xFF.5 , 1]", "{a: 0xFF.5\n b: 2}", "{a: 0xFF.5,}",
		"0xFF.5\t", "0xFF.5 ", "0xFF.5#", "{a: 0xFF.5{b:1}}", "[0xFF.5[1]]",
		"{a: 0xFF.5:2, b:3}", "{0xFF.5: 1}", "[0xFF.5\\n]", "{a: 0xFF.5\\}",
		"0x1.5e10", "0xFF.5e", "0xFFe5", "0xFF.5_", "0xFF_", "0x_",
	}
	changed := 0
	for _, src := range srcs {
		cur := probeRender(SafeParseData, src)
		dec := probeRender(probeSafeParseData, src)
		flag := "SAME"
		if cur != dec {
			flag = "CHANGED"
			changed++
		}
		t.Logf("%-24q current=%-30s decline=%-30s %s", src, cur, dec, flag)
	}
	t.Logf("TOTAL unicode/ender sources=%d changed=%d", len(srcs), changed)
}
