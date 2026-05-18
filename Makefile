.PHONY: all test vet fmt lint vuln clean cover cover-html cover-html-open \
        viz viz-tools viz-clean viz-index \
        viz-callvis viz-callgraph viz-goda viz-godepgraph \
        viz-gomod viz-golds viz-plantuml viz-list viz-modgraph

# Top-level Makefile for the whole AQL codebase.
#
# The repo is a collection of Go modules:
#
#   eng/go    — the kernel (parser, dispatch, types, signatures)
#   lang      — the language layer (native_* words, engine shim)
#   cmd/go    — CLI tools (aql, aqlweb, aqlwasm, genhelp)
#   calc      — small calculator built directly on eng
#   test/go   — shared TSV spec-runner scaffolding
#
# Each module has its own go.mod and a focused Makefile. The targets
# here fan out across the set so the whole codebase can be built,
# tested, visualised, and coverage-tracked from one place.

# Order matters for `make test`: eng must build before lang, etc.
MODULES := eng/go lang cmd/go calc test/go

all: test

# ---- per-module fan-out -------------------------------------------------

test:
	@set -e; for m in $(MODULES); do \
	  echo "==> test $$m"; \
	  ( cd $$m && go test ./... ); \
	done

vet:
	@set -e; for m in $(MODULES); do \
	  echo "==> vet $$m"; \
	  ( cd $$m && go vet ./... ); \
	done

fmt:
	@set -e; for m in $(MODULES); do \
	  echo "==> fmt $$m"; \
	  ( cd $$m && gofmt -w . ); \
	done

lint:
	@set -e; for m in $(MODULES); do \
	  echo "==> lint $$m"; \
	  ( cd $$m && golangci-lint run ./... ); \
	done

vuln:
	@set -e; for m in $(MODULES); do \
	  echo "==> vuln $$m"; \
	  ( cd $$m && govulncheck ./... ); \
	done

clean:
	@set -e; for m in $(MODULES); do \
	  echo "==> clean $$m"; \
	  ( cd $$m && go clean -testcache ); \
	done
	rm -rf $(VIZ_DIR) $(COVER_DIR)

# ---- coverage ----------------------------------------------------------
#
# Per-module coverage profiles land in $(COVER_DIR)/<module>.out. The
# `cover` target prints each module's totals plus an aggregate. The HTML
# variants render one report per module under $(COVER_DIR)/html/.

COVER_DIR := coverage

cover:
	@mkdir -p $(COVER_DIR)
	@set -e; for m in $(MODULES); do \
	  echo "==> cover $$m"; \
	  out="$(abspath $(COVER_DIR))/$$(echo $$m | tr '/' '_').out"; \
	  ( cd $$m && go test -coverprofile=$$out ./... ); \
	  ( cd $$m && go tool cover -func=$$out 2>/dev/null | tail -1 ) \
	    > "$(abspath $(COVER_DIR))/$$(echo $$m | tr '/' '_').total" 2>/dev/null || true; \
	done
	@echo
	@echo "==> per-module totals:"
	@for f in $(COVER_DIR)/*.out; do \
	  name=$$(basename $$f .out); \
	  total_file="$$(dirname $$f)/$$name.total"; \
	  if [ -s "$$total_file" ]; then \
	    total=$$(awk '/^total:/ {print $$3}' "$$total_file"); \
	  else total=N/A; fi; \
	  printf "  %-20s %s\n" "$$name" "$$total"; \
	done

cover-html: cover
	@mkdir -p $(COVER_DIR)/html
	@set -e; for m in $(MODULES); do \
	  name=$$(echo $$m | tr '/' '_'); \
	  f="$(abspath $(COVER_DIR))/$$name.out"; \
	  out="$(abspath $(COVER_DIR))/html/$$name.html"; \
	  [ -f "$$f" ] || continue; \
	  ( cd $$m && go tool cover -html=$$f -o $$out ) || true; \
	done
	@{ \
	  printf '<!doctype html>\n<html><head><meta charset="utf-8"><title>AQL coverage</title>'; \
	  printf '<style>body{font:14px system-ui;margin:2em;max-width:1000px}h1{margin-bottom:.4em}'; \
	  printf 'table{border-collapse:collapse;margin-top:1em}td,th{border:1px solid #ddd;padding:6px 12px;text-align:left}'; \
	  printf 'a{color:#06c;text-decoration:none}a:hover{text-decoration:underline}</style></head><body>'; \
	  printf '<h1>AQL coverage</h1>'; \
	  printf '<p>Generated %s</p>' "$$(date '+%Y-%m-%d %H:%M:%S')"; \
	  printf '<table><tr><th>Module</th><th>Coverage</th><th>Report</th></tr>'; \
	  for f in $(COVER_DIR)/*.out; do \
	    name=$$(basename $$f .out); \
	    total_file="$$(dirname $$f)/$$name.total"; \
	    if [ -s "$$total_file" ]; then \
	      total=$$(awk '/^total:/ {print $$3}' "$$total_file"); \
	    else total=N/A; fi; \
	    printf '<tr><td>%s</td><td>%s</td><td><a href="html/%s.html">view</a></td></tr>' "$$name" "$$total" "$$name"; \
	  done; \
	  printf '</table></body></html>'; \
	} > $(COVER_DIR)/index.html
	@echo "==> wrote $(COVER_DIR)/index.html"

# Open the combined coverage report. Tries `open` (macOS) then `xdg-open`.
cover-html-open: cover-html
	@(open $(COVER_DIR)/index.html 2>/dev/null || xdg-open $(COVER_DIR)/index.html 2>/dev/null || \
	  echo "open $(COVER_DIR)/index.html manually")

# ---- visualisation ----------------------------------------------------
#
# All viz output lands under $(VIZ_DIR)/<module>/. The viz targets run
# each Go-tool variant against every module so the whole codebase can be
# reviewed from one place. A top-level $(VIZ_DIR)/index.html aggregates
# every per-module index.html.

VIZ_DIR := viz

# Resolve GOBIN (where `go install` drops binaries). Honors $GOBIN, then
# $GOPATH/bin. Using absolute paths means we don't depend on $PATH.
GOBIN := $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell go env GOPATH)/bin
endif

CALLVIS    := $(GOBIN)/go-callvis
CALLGRAPH  := $(GOBIN)/callgraph
GODA       := $(GOBIN)/goda
GODEPGRAPH := $(GOBIN)/godepgraph
GOMOD      := $(GOBIN)/gomod
GOLDS      := $(GOBIN)/golds
GOPLANTUML := $(GOBIN)/goplantuml

# PlantUML renders the goplantuml-generated .puml to SVG. It's a Java jar,
# fetched once into ~/.cache/aql-viz and cached.
PLANTUML_JAR := $(HOME)/.cache/aql-viz/plantuml.jar
PLANTUML_URL := https://github.com/plantuml/plantuml/releases/latest/download/plantuml.jar

$(CALLVIS):
	go install github.com/ofabry/go-callvis@latest
$(CALLGRAPH):
	go install golang.org/x/tools/cmd/callgraph@latest
$(GODA):
	go install github.com/loov/goda@latest
$(GODEPGRAPH):
	go install github.com/kisielk/godepgraph@latest
$(GOMOD):
	go install github.com/Helcaraxan/gomod@latest
$(GOLDS):
	go install go101.org/golds@latest
$(GOPLANTUML):
	go install github.com/jfeliu007/goplantuml/cmd/goplantuml@latest
$(PLANTUML_JAR):
	@mkdir -p $(dir $(PLANTUML_JAR))
	curl -fsSL $(PLANTUML_URL) -o $(PLANTUML_JAR)

# Install every viz tool up front. Individual viz targets install on demand.
viz-tools: $(CALLVIS) $(CALLGRAPH) $(GODA) $(GODEPGRAPH) $(GOMOD) $(GOLDS) $(GOPLANTUML) $(PLANTUML_JAR)

# Per-module viz output directory.
mod_viz_dir = $(VIZ_DIR)/$(subst /,_,$1)

# Static call-graph (`-algo static`). Cluster + per-package detail SVGs.
# See VIZ-CALLGRAPH.md (if present) for the full pipeline; the awk
# functions are shared with the per-package split.
define CALLGRAPH_AWK_FUNCS
function pkg_of(name,    s, i, last_slash, dot_pos) {
  s = name; sub(/^\(\*?/, "", s); last_slash = 0;
  for (i = 1; i <= length(s); i++) if (substr(s, i, 1) == "/") last_slash = i;
  dot_pos = index(substr(s, last_slash + 1), ".");
  if (dot_pos == 0) return s;
  return substr(s, 1, last_slash + dot_pos - 1);
}
function leaf_of(name,    pkg, idx, p_len) {
  pkg = pkg_of(name); p_len = length(pkg) + 1;
  idx = index(name, pkg); if (idx == 0) return name;
  return substr(name, 1, idx - 1) substr(name, idx + p_len);
}
function pkg_leaf(p,    i, last_slash) {
  last_slash = 0;
  for (i = 1; i <= length(p); i++) if (substr(p, i, 1) == "/") last_slash = i;
  return (last_slash == 0) ? p : substr(p, last_slash + 1);
}
function is_exported(name,    leaf, t, j) {
  leaf = leaf_of(name);
  if (substr(leaf, 1, 1) == "(") {
    t = (substr(leaf, 2, 1) == "*") ? substr(leaf, 3) : substr(leaf, 2);
    if (substr(t, 1, 1) !~ /[A-Z]/) return 0;
    j = index(t, ").");
    if (j == 0) return 0;
    leaf = substr(t, j + 2);
  }
  return substr(leaf, 1, 1) ~ /[A-Z]/;
}
endef
export CALLGRAPH_AWK_FUNCS

define CALLGRAPH_MAIN_AWK
$(CALLGRAPH_AWK_FUNCS)
/^digraph/ || /^}/ { next }
{
  n_nodes = 0; copy = $$0;
  while (match(copy, /"[^"]+"/)) {
    name = substr(copy, RSTART + 1, RLENGTH - 2);
    n_nodes++; ln[n_nodes] = name;
    copy = substr(copy, RSTART + RLENGTH);
  }
  for (i = 1; i <= n_nodes; i++) {
    name = ln[i];
    if (!(name in seen) && is_exported(name)) {
      seen[name] = 1; p = pkg_of(name);
      pkgs[p] = pkgs[p] "    \"" name "\" [label=\"" leaf_of(name) "\" href=\"pkg_" pkg_leaf(p) ".svg\"];\n";
    }
  }
  if (/->/ && n_nodes == 2 && is_exported(ln[1]) && is_exported(ln[2])) {
    edges[++bn] = $$0;
  }
}
END {
  print "digraph callgraph {";
  print "  graph [compound=true, splines=true, rankdir=LR];";
  print "  node  [shape=box, fontsize=9, style=filled, fillcolor=\"#f8f8f8\"];";
  print "  edge  [color=\"#888\", arrowsize=0.6];";
  ci = 0;
  for (p in pkgs) {
    ci++;
    printf "  subgraph cluster_%d {\n    label=\"%s\";\n    style=\"rounded,filled\"; color=\"#bbb\"; fillcolor=\"#fafafa\";\n    href=\"pkg_%s.svg\";\n%s  }\n", ci, p, pkg_leaf(p), pkgs[p];
  }
  for (i = 1; i <= bn; i++) print "  " edges[i];
  print "}";
}
endef
export CALLGRAPH_MAIN_AWK

define CALLGRAPH_PERPKG_AWK
$(CALLGRAPH_AWK_FUNCS)
/^digraph/ || /^}/ { next }
{
  n_nodes = 0; copy = $$0;
  while (match(copy, /"[^"]+"/)) {
    name = substr(copy, RSTART + 1, RLENGTH - 2);
    n_nodes++; ln[n_nodes] = name;
    copy = substr(copy, RSTART + RLENGTH);
  }
  for (i = 1; i <= n_nodes; i++) {
    name = ln[i];
    if (!(name in seen)) {
      seen[name] = 1; p = pkg_of(name);
      pkg_nodes[p] = pkg_nodes[p] "    \"" name "\" [label=\"" leaf_of(name) "\"];\n";
    }
  }
  if (/->/ && n_nodes == 2 && pkg_of(ln[1]) == pkg_of(ln[2])) {
    p = pkg_of(ln[1]);
    pkg_edges[p] = pkg_edges[p] "  " $$0 "\n";
  }
}
END {
  for (p in pkg_nodes) {
    out = out_dir "/pkg_" pkg_leaf(p) ".dot";
    print "digraph pkg {" > out;
    print "  graph [splines=true, rankdir=LR, labelloc=t];" > out;
    print "  label=\"" p "\";" > out;
    print "  node  [shape=box, fontsize=9, style=filled, fillcolor=\"#f8f8f8\"];" > out;
    print "  edge  [color=\"#888\", arrowsize=0.6];" > out;
    printf "%s", pkg_nodes[p] > out;
    if (p in pkg_edges) printf "%s", pkg_edges[p] > out;
    print "}" > out;
    close(out);
  }
}
endef
export CALLGRAPH_PERPKG_AWK

CALLGRAPH_LAYOUT ?= dot

viz-callgraph: $(CALLGRAPH)
	@set -e; for m in $(MODULES); do \
	  d=$(VIZ_DIR)/$$(echo $$m | tr '/' '_'); \
	  mkdir -p $$d; rm -f $$d/pkg_*.dot $$d/pkg_*.svg; \
	  mod=$$(cd $$m && go list -m); \
	  echo "==> callgraph $$m ($$mod)"; \
	  ( cd $$m && $(CALLGRAPH) -algo static -format=graphviz ./... ) > $$d/callgraph.full.dot || \
	    { echo "  (callgraph skipped for $$m)"; continue; }; \
	  awk -v m="$$mod" ' \
	    { n = gsub(m, "&") } \
	    /^digraph/ || /^}/ { print; next } \
	    /->/ && n >= 2 { print; next } \
	    !/->/ && n >= 1 { print } \
	  ' $$d/callgraph.full.dot > $$d/callgraph.filtered.dot; \
	  awk "$$CALLGRAPH_MAIN_AWK" $$d/callgraph.filtered.dot > $$d/callgraph.dot; \
	  awk -v out_dir=$$d "$$CALLGRAPH_PERPKG_AWK" $$d/callgraph.filtered.dot; \
	  for f in $$d/callgraph.dot $$d/pkg_*.dot; do \
	    [ -f "$$f" ] || continue; \
	    $(CALLGRAPH_LAYOUT) -Tsvg "$$f" -o "$${f%.dot}.svg" 2>/dev/null || echo "  (dot missing for $$f)"; \
	  done; \
	done

viz-goda: $(GODA)
	@set -e; for m in $(MODULES); do \
	  d=$(VIZ_DIR)/$$(echo $$m | tr '/' '_'); mkdir -p $$d; \
	  echo "==> goda $$m"; \
	  ( cd $$m && $(GODA) graph ./... ) > $$d/goda.dot || continue; \
	  dot -Tsvg $$d/goda.dot -o $$d/goda.svg 2>/dev/null || true; \
	done

viz-godepgraph: $(GODEPGRAPH)
	@set -e; for m in $(MODULES); do \
	  d=$(VIZ_DIR)/$$(echo $$m | tr '/' '_'); mkdir -p $$d; \
	  mod=$$(cd $$m && go list -m); \
	  echo "==> godepgraph $$m ($$mod)"; \
	  ( cd $$m && $(GODEPGRAPH) -s $$mod ) > $$d/godepgraph.dot || continue; \
	  dot -Tsvg $$d/godepgraph.dot -o $$d/godepgraph.svg 2>/dev/null || true; \
	done

viz-gomod: $(GOMOD)
	@set -e; for m in $(MODULES); do \
	  d=$(VIZ_DIR)/$$(echo $$m | tr '/' '_'); mkdir -p $$d; \
	  echo "==> gomod $$m"; \
	  ( cd $$m && $(GOMOD) graph ) > $$d/gomod.dot || continue; \
	  dot -Tsvg $$d/gomod.dot -o $$d/gomod.svg 2>/dev/null || true; \
	done

viz-plantuml: $(GOPLANTUML) $(PLANTUML_JAR)
	@set -e; for m in $(MODULES); do \
	  d=$(VIZ_DIR)/$$(echo $$m | tr '/' '_'); mkdir -p $$d; \
	  echo "==> plantuml $$m"; \
	  ( cd $$m && $(GOPLANTUML) -recursive \
	    -show-aggregations -show-compositions -show-implementations \
	    -show-aliases -show-connection-labels \
	    -aggregate-private-members . ) > $$d/uml.puml 2>/dev/null || continue; \
	  java -jar $(PLANTUML_JAR) -tsvg -o $$(realpath $$d) $$d/uml.puml 2>/dev/null || true; \
	done

viz-list:
	@set -e; for m in $(MODULES); do \
	  d=$(VIZ_DIR)/$$(echo $$m | tr '/' '_'); mkdir -p $$d; \
	  ( cd $$m && go list -deps -json ./... ) > $$d/deps.json; \
	done

viz-modgraph:
	@set -e; for m in $(MODULES); do \
	  d=$(VIZ_DIR)/$$(echo $$m | tr '/' '_'); mkdir -p $$d; \
	  ( cd $$m && go mod graph ) > $$d/mod.graph; \
	done

# Build a top-level index.html that embeds every SVG under $(VIZ_DIR).
# SVGs are inlined (stripped of their XML prolog) and wired through
# svg-pan-zoom for drag-pan + scroll-zoom inside their 80vh frames.
# A nav at the top groups SVGs by module.
viz-index:
	@mkdir -p $(VIZ_DIR)
	@{ \
	  printf '<!doctype html>\n<html><head><meta charset="utf-8">'; \
	  printf '<title>AQL — code structure</title>'; \
	  printf '<style>'; \
	  printf 'body{font:14px system-ui;margin:2em;max-width:1400px;color:#222}'; \
	  printf 'h1{margin-bottom:.2em}'; \
	  printf 'h2{margin-top:2em;padding-top:1em;border-top:1px solid #ddd}'; \
	  printf 'h3{margin-top:1.5em;color:#555}'; \
	  printf 'nav{margin:1em 0}nav details{margin-bottom:.4em}nav a{display:inline-block;margin-right:1em}'; \
	  printf '.frame{height:80vh;border:1px solid #ddd;background:#fff;overflow:hidden;position:relative;touch-action:none}'; \
	  printf '.frame > svg{width:100%%;height:100%%;display:block;cursor:grab}'; \
	  printf '.frame > svg:active{cursor:grabbing}'; \
	  printf '.meta{color:#888;font-size:.9em}'; \
	  printf 'a{color:#06c}'; \
	  printf '</style>'; \
	  printf '<script src="https://cdn.jsdelivr.net/npm/svg-pan-zoom@3.6.1/dist/svg-pan-zoom.min.js"></script>'; \
	  printf '</head><body>'; \
	  printf '<h1>AQL — code structure</h1>'; \
	  printf '<p class="meta">Generated %s · drag to pan, scroll/pinch to zoom, double-click to reset</p>' "$$(date '+%Y-%m-%d %H:%M:%S')"; \
	  printf '<nav>'; \
	  for d in $(VIZ_DIR)/*/; do \
	    [ -d "$$d" ] || continue; \
	    name=$$(basename $$d); \
	    printf '<details><summary>%s</summary>' "$$name"; \
	    for f in $$d*.svg; do \
	      [ -f "$$f" ] || continue; \
	      svg=$$(basename "$$f"); \
	      printf '<a href="#%s-%s">%s</a>' "$$name" "$$svg" "$$svg"; \
	    done; \
	    printf '</details>'; \
	  done; \
	  printf '</nav>'; \
	  for d in $(VIZ_DIR)/*/; do \
	    [ -d "$$d" ] || continue; \
	    name=$$(basename $$d); \
	    printf '<h2>%s</h2>' "$$name"; \
	    for f in $$d*.svg; do \
	      [ -f "$$f" ] || continue; \
	      svg=$$(basename "$$f"); \
	      printf '<section id="%s-%s"><h3><a href="%s">%s</a></h3><div class="frame">' "$$name" "$$svg" "$$f" "$$svg"; \
	      sed -n '/<svg/,$$p' "$$f"; \
	      printf '</div></section>'; \
	    done; \
	  done; \
	  printf '<script>'; \
	  printf 'window.addEventListener("load", function () {'; \
	  printf '  document.querySelectorAll(".frame > svg").forEach(function (svg) {'; \
	  printf '    svg.removeAttribute("width"); svg.removeAttribute("height");'; \
	  printf '    svg.setAttribute("width", "100%%"); svg.setAttribute("height", "100%%");'; \
	  printf '    var pz = svgPanZoom(svg, {zoomEnabled:true, controlIconsEnabled:true, fit:true, center:true, minZoom:0.05, maxZoom:50});'; \
	  printf '    svg.addEventListener("dblclick", function () { pz.reset(); });'; \
	  printf '  });'; \
	  printf '});'; \
	  printf '</script></body></html>'; \
	} > $(VIZ_DIR)/index.html
	@echo "Wrote $(VIZ_DIR)/index.html — open with: open $(VIZ_DIR)/index.html"

# Run every viz target that works for library modules (skips viz-callvis
# which needs a single main package, and viz-golds which blocks on a server).
viz: viz-callgraph viz-goda viz-godepgraph viz-gomod viz-plantuml viz-list viz-modgraph viz-index
	@echo "Wrote artifacts under $(VIZ_DIR)/"

viz-clean:
	rm -rf $(VIZ_DIR)

# Single-package interactive call viewer; needs CALLVIS_PKG pointing at
# a directory with a main package. Default: cmd/go/aql.
CALLVIS_PKG ?= ./cmd/go/aql
viz-callvis: $(CALLVIS)
	@mkdir -p $(VIZ_DIR)
	$(CALLVIS) -file $(VIZ_DIR)/callvis -format svg $(CALLVIS_PKG)

# Local code browser. Blocks; open http://localhost:56789 in a browser.
GOLDS_PKG ?= ./...
viz-golds: $(GOLDS)
	@cd lang && $(GOLDS) -port=56789 $(GOLDS_PKG)
