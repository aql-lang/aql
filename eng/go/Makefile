.PHONY: test vet fmt lint vuln clean \
        viz viz-tools viz-clean viz-index \
        viz-callvis viz-callgraph viz-goda viz-godepgraph \
        viz-gomod viz-golds viz-plantuml viz-list viz-modgraph

# This module (github.com/aql-lang/aql/eng) is the AQL engine
# kernel + jsonic parser + kernel spec runner. It is standalone — it has
# no dependency on the lang or cmd/go modules.

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

# golangci-lint (config: ../../.golangci.yml, found by walking up).
lint:
	golangci-lint run ./...

vuln:
	govulncheck ./...

clean:
	go clean -testcache

# ---- Code structure visualization ----------------------------------------
#
# Artifacts land in $(VIZ_DIR). Override the package set with PKG=...
# Most graph targets pipe DOT through Graphviz (`dot`); install it via
# `brew install graphviz` if missing.

VIZ_DIR := viz
PKG     ?= ./...
MODULE  := $(shell go list -m)

# go-callvis can only graph one main package at a time. With -tests + ./...
# every package's test binary becomes its own main, so callvis needs a
# narrower default. Override with: make viz-callvis CALLVIS_PKG=./parser
CALLVIS_PKG ?= .

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

# Install every viz tool up front. Optional — individual viz targets
# install their own tool on demand.
viz-tools: $(CALLVIS) $(CALLGRAPH) $(GODA) $(GODEPGRAPH) $(GOMOD) $(GOLDS) $(GOPLANTUML) $(PLANTUML_JAR)

# Interactive call-graph viewer. Wants exactly one main package. With -tests
# the package's test binary acts as that main. Override CALLVIS_PKG to graph
# a different package; for a real main, omit -tests.
viz-callvis: $(CALLVIS)
	@mkdir -p $(VIZ_DIR)
	$(CALLVIS) -file $(VIZ_DIR)/callvis -format svg -tests $(CALLVIS_PKG)

# Static call-graph (`-algo static` — only lexically-visible calls, fewer
# noise edges than CHA's virtual-dispatch over-approximation).
#
# Output layout:
#   callgraph.svg     — bird's-eye view: exported nodes only, clustered by
#                       Go package, each cluster links to its detail SVG.
#   pkg_<leaf>.svg    — drill-down per package: every node (incl. unexported)
#                       and every intra-package edge.
#   index.html (via viz-index) embeds them with svg-pan-zoom.
#
# Pipeline:
#   1. callgraph              → callgraph.full.dot     (raw, with stdlib)
#   2. project-only filter    → callgraph.filtered.dot (drops runtime/stdlib)
#   3. cluster + exported     → callgraph.dot          (bird's-eye)
#   4. per-package split      → pkg_*.dot              (one per package)
#   5. $(CALLGRAPH_LAYOUT) renders each .dot.
#
# `dot` (hierarchical) handles clusters cleanly and renders ~5x faster
# than sfdp on graphs of this size — override with CALLGRAPH_LAYOUT=sfdp
# for a force-directed constellation instead.
CALLGRAPH_LAYOUT ?= dot

# The cluster/exported/href awk and the per-package split awk share three
# string functions (package extraction etc.). Defining them once in a
# `define` block keeps the recipe readable.
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

# Cluster + exported-only + href on each node (and cluster) pointing at the
# per-package detail SVG.
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

# Per-package split: for each package observed, write a pkg_<leaf>.dot
# containing all of that package's nodes plus every intra-package edge.
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

viz-callgraph: $(CALLGRAPH)
	@mkdir -p $(VIZ_DIR)
	@rm -f $(VIZ_DIR)/pkg_*.dot $(VIZ_DIR)/pkg_*.svg
	$(CALLGRAPH) -algo static -format=graphviz $(PKG) > $(VIZ_DIR)/callgraph.full.dot
	@awk -v m='$(MODULE)' ' \
	  { n = gsub(m, "&") } \
	  /^digraph/ || /^}/ { print; next } \
	  /->/ && n >= 2 { print; next } \
	  !/->/ && n >= 1 { print } \
	' $(VIZ_DIR)/callgraph.full.dot > $(VIZ_DIR)/callgraph.filtered.dot
	@awk "$$CALLGRAPH_MAIN_AWK" $(VIZ_DIR)/callgraph.filtered.dot > $(VIZ_DIR)/callgraph.dot
	@awk -v out_dir='$(VIZ_DIR)' "$$CALLGRAPH_PERPKG_AWK" $(VIZ_DIR)/callgraph.filtered.dot
	@printf 'callgraph: raw %s -> filtered %s -> clustered %s lines; %s per-pkg detail file(s)\n' \
	  $$(wc -l < $(VIZ_DIR)/callgraph.full.dot) \
	  $$(wc -l < $(VIZ_DIR)/callgraph.filtered.dot) \
	  $$(wc -l < $(VIZ_DIR)/callgraph.dot) \
	  $$(ls $(VIZ_DIR)/pkg_*.dot 2>/dev/null | wc -l | tr -d ' ')
	@for f in $(VIZ_DIR)/callgraph.dot $(VIZ_DIR)/pkg_*.dot; do \
	  [ -f "$$f" ] || continue; \
	  $(CALLGRAPH_LAYOUT) -Tsvg "$$f" -o "$${f%.dot}.svg"; \
	done

# Package dependency graph via goda.
viz-goda: $(GODA)
	@mkdir -p $(VIZ_DIR)
	$(GODA) graph "$(PKG)" > $(VIZ_DIR)/goda.dot
	dot -Tsvg $(VIZ_DIR)/goda.dot -o $(VIZ_DIR)/goda.svg

# Package import graph rooted at this module.
viz-godepgraph: $(GODEPGRAPH)
	@mkdir -p $(VIZ_DIR)
	$(GODEPGRAPH) -s $(MODULE) > $(VIZ_DIR)/godepgraph.dot
	dot -Tsvg $(VIZ_DIR)/godepgraph.dot -o $(VIZ_DIR)/godepgraph.svg

# Module dependency graph (third-party deps).
viz-gomod: $(GOMOD)
	@mkdir -p $(VIZ_DIR)
	$(GOMOD) graph > $(VIZ_DIR)/gomod.dot
	dot -Tsvg $(VIZ_DIR)/gomod.dot -o $(VIZ_DIR)/gomod.svg

# Local code browser. Blocks; open http://localhost:56789 in a browser.
viz-golds: $(GOLDS)
	$(GOLDS) -port=56789 $(PKG)

# PlantUML class diagram from Go structs/interfaces. Renders straight to
# SVG via plantuml.jar so the index page can embed it.
#
# goplantuml flags: by default it draws only fields/methods inside class
# boxes — every relationship arrow (aggregation, composition, interface
# impl) needs an explicit flag. We turn them all on plus
# -aggregate-private-members so unexported fields contribute too.
viz-plantuml: $(GOPLANTUML) $(PLANTUML_JAR)
	@mkdir -p $(VIZ_DIR)
	$(GOPLANTUML) -recursive \
	  -show-aggregations \
	  -show-compositions \
	  -show-implementations \
	  -show-aliases \
	  -show-connection-labels \
	  -aggregate-private-members \
	  . > $(VIZ_DIR)/uml.puml
	java -jar $(PLANTUML_JAR) -tsvg -o $(abspath $(VIZ_DIR)) $(VIZ_DIR)/uml.puml

# Built-in: full transitive dep list as JSON.
viz-list:
	@mkdir -p $(VIZ_DIR)
	go list -deps -json $(PKG) > $(VIZ_DIR)/deps.json

# Built-in: `go mod graph` raw output.
viz-modgraph:
	@mkdir -p $(VIZ_DIR)
	go mod graph > $(VIZ_DIR)/mod.graph

# Build an HTML index that embeds every SVG in $(VIZ_DIR). SVGs are inlined
# (stripped of their XML prolog) and wired through svg-pan-zoom so each one
# is draggable and zoomable inside its 80vh frame.
viz-index:
	@mkdir -p $(VIZ_DIR)
	@{ \
	  printf '%s\n' \
	    '<!doctype html>' \
	    '<html><head><meta charset="utf-8">' \
	    '<title>$(MODULE) — code structure</title>' \
	    '<style>' \
	    'body{font:14px system-ui;margin:2em;max-width:1400px;color:#222}' \
	    'h1{margin-bottom:.2em}' \
	    'h2{margin-top:2em;padding-top:1em;border-top:1px solid #ddd}' \
	    'nav{margin:1em 0}nav a{display:inline-block;margin-right:1em}' \
	    '.frame{height:80vh;border:1px solid #ddd;background:#fff;overflow:hidden;position:relative;touch-action:none}' \
	    '.frame > svg{width:100%;height:100%;display:block;cursor:grab}' \
	    '.frame > svg:active{cursor:grabbing}' \
	    '.meta{color:#888;font-size:.9em}' \
	    'a{color:#06c}' \
	    '</style>' \
	    '<script src="https://cdn.jsdelivr.net/npm/svg-pan-zoom@3.6.1/dist/svg-pan-zoom.min.js"></script>' \
	    '</head><body>'; \
	  printf '<h1>$(MODULE)</h1>\n'; \
	  printf '<p class="meta">Generated %s · drag to pan, scroll/pinch to zoom, double-click to reset</p>\n' "$$(date '+%Y-%m-%d %H:%M:%S')"; \
	  printf '<nav>'; \
	  for f in $(VIZ_DIR)/*.svg; do \
	    [ -f "$$f" ] || continue; \
	    name=$$(basename "$$f"); \
	    printf '<a href="#%s">%s</a>' "$$name" "$$name"; \
	  done; \
	  printf '</nav>\n'; \
	  for f in $(VIZ_DIR)/*.svg; do \
	    [ -f "$$f" ] || continue; \
	    name=$$(basename "$$f"); \
	    printf '<section id="%s"><h2><a href="%s">%s</a></h2><div class="frame">\n' "$$name" "$$name" "$$name"; \
	    sed -n '/<svg/,$$p' "$$f"; \
	    printf '</div></section>\n'; \
	  done; \
	  printf '%s\n' \
	    '<script>' \
	    'window.addEventListener("load", function () {' \
	    '  document.querySelectorAll(".frame > svg").forEach(function (svg) {' \
	    '    svg.removeAttribute("width");' \
	    '    svg.removeAttribute("height");' \
	    '    svg.setAttribute("width", "100%");' \
	    '    svg.setAttribute("height", "100%");' \
	    '    var pz = svgPanZoom(svg, {' \
	    '      zoomEnabled: true, controlIconsEnabled: true,' \
	    '      fit: true, center: true, minZoom: 0.05, maxZoom: 50' \
	    '    });' \
	    '    svg.addEventListener("dblclick", function () { pz.reset(); });' \
	    '  });' \
	    '});' \
	    '</script>' \
	    '</body></html>'; \
	} > $(VIZ_DIR)/index.html
	@echo "Wrote $(VIZ_DIR)/index.html — open with: open $(VIZ_DIR)/index.html"

# Run everything that works for a library module (skips viz-callvis,
# which needs a main, and viz-golds, which blocks on a server).
viz: viz-callgraph viz-goda viz-godepgraph viz-gomod viz-plantuml viz-list viz-modgraph viz-index
	@echo "Wrote artifacts to $(VIZ_DIR)/"

viz-clean:
	rm -rf $(VIZ_DIR)
