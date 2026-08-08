package basic_test

// The Go half of the base-layer parity corpus (basic/spec/*.tsv).
//
// basic/ts/src/basicspec.test.ts is the other half. The two runners share
// NO code — they read the same files and each builds the registry and
// renders the residual independently, exactly as core/spec's and
// parser/spec's pairs do. That independence is the point: shared
// scaffolding can hide the same bug from both engines
// (design/CORE-GO-TS-DEFECTS.0.md, blind spot 9).
//
// This is a SPEC, not a differential: the `expected` column is the
// documented contract, so a row can legitimately fail on BOTH engines —
// which is exactly the class of defect an agreement-only corpus is
// structurally blind to.

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	basic "github.com/boru-lang/boru/basic/go"
	core "github.com/boru-lang/boru/core/go"
)

const basicSpecDir = "../spec"

type basicSpecRow struct {
	file     string
	line     int
	prog     string
	expected string
	note     string
}

func parseBasicSpec(t *testing.T, path string) []basicSpecRow {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	var rows []basicSpecRow
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		// A line is a COMMENT only when it starts with '#' AND carries no
		// tab — '#' is boru's own comment marker, so a program may begin
		// with one.
		if strings.TrimSpace(line) == "" ||
			(strings.HasPrefix(line, "#") && !strings.Contains(line, "\t")) {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			t.Fatalf("%s:%d: want at least 2 tab-separated columns, got %d", path, n, len(parts))
		}
		row := basicSpecRow{file: filepath.Base(path), line: n, prog: parts[0], expected: parts[1]}
		if len(parts) > 2 {
			row.note = parts[2]
		}
		rows = append(rows, row)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return rows
}

// basicSpecToken builds one program token: a decimal is an Integer, '…' is
// a String, anything else is a Word. Deliberately tiny — the corpus is
// about the WORDS, so the token notation stays out of the way.
func basicSpecToken(tok string) core.Value {
	if strings.HasPrefix(tok, "'") && strings.HasSuffix(tok, "'") && len(tok) >= 2 {
		return core.NewString(tok[1 : len(tok)-1])
	}
	if n, err := strconv.ParseInt(tok, 10, 64); err == nil {
		return core.NewInteger(n)
	}
	return core.NewWord(tok)
}

// basicSpecRegistry registers ONLY the stack vocabulary, so a row's result
// depends on nothing else in the layer.
func basicSpecRegistry(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	for _, nf := range basic.StackNatives {
		r.RegisterNativeFunc(nf)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	return r
}

func TestBasicSpec(t *testing.T) {
	entries, err := os.ReadDir(basicSpecDir)
	if err != nil {
		t.Fatalf("read %s: %v", basicSpecDir, err)
	}
	total := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		path := filepath.Join(basicSpecDir, e.Name())
		rows := parseBasicSpec(t, path)
		t.Run(strings.TrimSuffix(e.Name(), ".tsv"), func(t *testing.T) {
			for _, row := range rows {
				total++
				var prog []core.Value
				for _, tok := range strings.Fields(row.prog) {
					prog = append(prog, basicSpecToken(tok))
				}
				got := ""
				out, rerr := core.NewTop(basicSpecRegistry(t)).Run(prog)
				if rerr != nil {
					var be *core.BoruError
					if errors.As(rerr, &be) {
						got = "ERROR:" + be.Code
					} else {
						got = "ERROR:non_boru:" + rerr.Error()
					}
				} else {
					parts := make([]string, 0, len(out))
					for _, v := range out {
						parts = append(parts, core.CanonValue(v))
					}
					got = strings.Join(parts, " ")
				}
				if got != row.expected {
					t.Errorf("%s:%d  %s\n  got  %q\n  want %q\n  (%s)",
						row.file, row.line, row.prog, got, row.expected, row.note)
				}
			}
		})
	}
	if total == 0 {
		t.Fatal("basic/spec produced no rows — the corpus is not being read")
	}
	t.Logf("basic/spec: %d rows", total)
}
