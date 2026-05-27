// parse.go implements the `+`-separated argv grammar used by
// `aql serve`. Each segment is `<svc> [flags...]` and is forwarded
// verbatim to that service's own flag parser, so adding a new
// service requires no parser changes here.

package serve

// splitSegments splits argv on bare "+" tokens. Empty segments
// (caused by leading, trailing, or doubled "+" tokens) are dropped
// so callers can rely on every returned segment being non-empty.
//
//	["registry","-r","./mods","+","lsp","-p","9000"]
//	→ [["registry","-r","./mods"], ["lsp","-p","9000"]]
func splitSegments(args []string) [][]string {
	var out [][]string
	var cur []string
	for _, a := range args {
		if a == "+" {
			if len(cur) > 0 {
				out = append(out, cur)
				cur = nil
			}
			continue
		}
		cur = append(cur, a)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}
