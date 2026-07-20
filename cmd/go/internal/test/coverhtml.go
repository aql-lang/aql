package test

import (
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
)

// mkdirAll and writeFile are test seams (design/TEST-SEAMS.10.md) over the os
// functions, so the HTML writer's I/O-error arms — unreachable when writing to
// a temp dir under the test's root uid — can be driven.
var (
	mkdirAll  = os.MkdirAll
	writeFile = os.WriteFile
)

// covAccum aggregates per-module line coverage across every suite in a run.
// Each suite runs on its OWN registry, so its coverage is folded in here
// (covered rows unioned across suites; the denominator is the module's stable
// executable-row set) and reported / rendered once at the end. Without this a
// module imported by two suites would be reported twice with partial coverage.
type covAccum struct {
	order   []string                // module ids, first-seen order
	source  map[string]string       // id -> module source text
	covered map[string]map[int]bool // id -> rows that ran
	denom   map[string]map[int]bool // id -> executable rows
}

func newCovAccum() *covAccum {
	return &covAccum{
		source:  map[string]string{},
		covered: map[string]map[int]bool{},
		denom:   map[string]map[int]bool{},
	}
}

// collect folds one finished suite's registry coverage into the accumulator.
func (a *covAccum) collect(reg *native.Registry) {
	for _, id := range reg.CoverSourceIDs() {
		covered, uncovered, _ := modules.CoverageFor(reg, id)
		src, _ := reg.CoverSource(id)
		a.add(id, src, covered, uncovered)
	}
}

// add merges one module's covered/uncovered rows: uncovered rows extend the
// denominator, covered rows extend both. First sight of an id records its
// source and first-seen order.
func (a *covAccum) add(id, src string, covered, uncovered []int) {
	if _, seen := a.denom[id]; !seen {
		a.order = append(a.order, id)
		a.source[id] = src
		a.covered[id] = map[int]bool{}
		a.denom[id] = map[int]bool{}
	}
	for _, r := range covered {
		a.covered[id][r] = true
		a.denom[id][r] = true
	}
	for _, r := range uncovered {
		a.denom[id][r] = true
	}
}

// rows returns the sorted covered and uncovered row slices for a module.
func (a *covAccum) rows(id string) (covered, uncovered []int) {
	for r := range a.denom[id] {
		if a.covered[id][r] {
			covered = append(covered, r)
		} else {
			uncovered = append(uncovered, r)
		}
	}
	sort.Ints(covered)
	sort.Ints(uncovered)
	return covered, uncovered
}

// aggregate sums covered and total executable lines across every module — the
// denominator for the --coverage-min gate.
func (a *covAccum) aggregate() (covered, total int) {
	for _, id := range a.order {
		cov, unc := a.rows(id)
		covered += len(cov)
		total += len(cov) + len(unc)
	}
	return covered, total
}

// report prints the aggregated per-module summary line (with uncovered rows)
// for every module the run touched.
func (a *covAccum) report(w io.Writer) {
	for _, id := range a.order {
		covered, uncovered := a.rows(id)
		fmt.Fprint(w, coverageLine(id, len(covered), len(covered)+len(uncovered), uncovered))
	}
}

// indexRow is one module's entry on the coverage index page.
type indexRow struct {
	id, page       string
	covered, total int
}

// writeHTML renders the coverage report tree under dir (index.html + one page
// per module, source lines coloured by coverage) and returns the index path.
// Returns ("", nil) when no module was covered — nothing to report.
func (a *covAccum) writeHTML(dir string) (string, error) {
	if len(a.order) == 0 {
		return "", nil
	}
	if err := mkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	rows := make([]indexRow, 0, len(a.order))
	for i, id := range a.order {
		covered, uncovered := a.rows(id)
		page := fmt.Sprintf("mod%d.html", i)
		body := renderModuleHTML(id, a.source[id], covered, uncovered)
		if err := writeFile(filepath.Join(dir, page), []byte(body), 0o644); err != nil {
			return "", err
		}
		rows = append(rows, indexRow{id: id, page: page, covered: len(covered), total: len(covered) + len(uncovered)})
	}
	index := filepath.Join(dir, "index.html")
	if err := writeFile(index, []byte(renderIndexHTML(rows)), 0o644); err != nil {
		return "", err
	}
	return index, nil
}

// renderModuleHTML renders one module's source with each executable line
// coloured green (covered) or red (uncovered) and non-executable lines plain.
func renderModuleHTML(id, src string, covered, uncovered []int) string {
	cov, unc := toSet(covered), toSet(uncovered)
	var b strings.Builder
	b.WriteString(pageCSS)
	total := len(covered) + len(uncovered)
	fmt.Fprintf(&b, "<title>coverage &mdash; %s</title></head>\n<body>\n"+
		"<header><h1>%s</h1><p>%s covered &mdash; %d / %d lines</p>"+
		"<p><a href=\"index.html\">&larr; all modules</a></p></header>\n<main>\n",
		html.EscapeString(id), html.EscapeString(id), pctString(len(covered), total), len(covered), total)
	for i, text := range strings.Split(src, "\n") {
		ln := i + 1
		cls := "line"
		switch {
		case cov[ln]:
			cls = "line covered"
		case unc[ln]:
			cls = "line uncovered"
		}
		fmt.Fprintf(&b, "<div class=\"%s\"><span class=\"ln\">%d</span><span class=\"code\">%s</span></div>\n",
			cls, ln, html.EscapeString(text))
	}
	b.WriteString("</main></body></html>\n")
	return b.String()
}

// renderIndexHTML renders the coverage index: one linked row per module.
func renderIndexHTML(rows []indexRow) string {
	var b strings.Builder
	b.WriteString(indexCSS)
	b.WriteString("<title>coverage report</title></head>\n<body>\n<header><h1>coverage report</h1></header>\n" +
		"<main><table><thead><tr><th>module</th><th>coverage</th><th>lines</th></tr></thead><tbody>\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "<tr><td><a href=\"%s\">%s</a></td><td>%s</td><td>%d / %d</td></tr>\n",
			html.EscapeString(r.page), html.EscapeString(r.id), pctString(r.covered, r.total), r.covered, r.total)
	}
	b.WriteString("</tbody></table></main></body></html>\n")
	return b.String()
}

// toSet builds a membership set from a row-number slice.
func toSet(xs []int) map[int]bool {
	m := make(map[int]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// pctString formats a coverage percentage.
func pctString(covered, total int) string {
	return fmt.Sprintf("%.1f%%", percent(covered, total))
}

// pageCSS is the static <head> (doctype + inline, theme-aware stylesheet) of a
// module page, up to but not including its <title>. Written raw (not through a
// format string) so the CSS's own characters are never treated as verbs.
const pageCSS = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<style>
:root{--bg:#fff;--fg:#1a1a1a;--mut:#999;--ln:#f4f4f4;--cov:#e6ffed;--covln:#cdffd8;--unc:#ffeef0;--uncln:#ffd7dc;--bar:#f5f5f5;--bd:#e2e2e2}
@media(prefers-color-scheme:dark){:root{--bg:#0d1117;--fg:#e6edf3;--mut:#8b949e;--ln:#161b22;--cov:#122419;--covln:#1c3a28;--unc:#2d1417;--uncln:#4a1d23;--bar:#161b22;--bd:#30363d}}
*{box-sizing:border-box}
body{margin:0;font:13px/1.6 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;background:var(--bg);color:var(--fg)}
header{padding:14px 20px;background:var(--bar);border-bottom:1px solid var(--bd)}
header h1{margin:0 0 4px;font-size:15px;font-weight:600;word-break:break-all}
header p{margin:2px 0;color:var(--mut)}
header a{color:inherit}
main{padding:6px 0}
.line{display:flex;white-space:pre-wrap;word-break:break-word}
.ln{flex:0 0 4em;text-align:right;padding:0 12px;color:var(--mut);background:var(--ln);user-select:none;border-right:1px solid var(--bd)}
.code{padding:0 12px;flex:1;min-width:0}
.covered{background:var(--cov)}
.covered .ln{background:var(--covln)}
.uncovered{background:var(--unc)}
.uncovered .ln{background:var(--uncln)}
</style>`

// indexCSS is the static <head> of the index page.
const indexCSS = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<style>
:root{--bg:#fff;--fg:#1a1a1a;--mut:#666;--bar:#f5f5f5;--bd:#e2e2e2;--row:#fafafa}
@media(prefers-color-scheme:dark){:root{--bg:#0d1117;--fg:#e6edf3;--mut:#8b949e;--bar:#161b22;--bd:#30363d;--row:#161b22}}
body{margin:0;font:14px/1.5 system-ui,-apple-system,Segoe UI,Roboto,sans-serif;background:var(--bg);color:var(--fg)}
header{padding:16px 24px;background:var(--bar);border-bottom:1px solid var(--bd)}
header h1{margin:0;font-size:18px}
main{padding:16px 24px}
table{border-collapse:collapse;width:100%;max-width:900px}
th,td{text-align:left;padding:8px 12px;border-bottom:1px solid var(--bd)}
th{color:var(--mut);font-weight:600;font-size:12px;text-transform:uppercase;letter-spacing:.04em}
tbody tr:nth-child(even){background:var(--row)}
td a{color:inherit;text-decoration:none;font-family:ui-monospace,monospace;word-break:break-all}
td a:hover{text-decoration:underline}
</style>`
