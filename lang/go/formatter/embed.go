package formatter

import "strings"

// Embedded BORU in host documents (Markdown, HTML, or anything that
// tolerates HTML comments) is delimited two ways, both handled here:
//
//   - Markdown fenced code blocks whose info string is "boru":
//     ```boru
//     def  x  1
//     ```
//   - explicit marker comments, for HTML or any non-fenced context:
//     <!-- borufmt -->
//     def  x  1
//     <!-- /borufmt -->
//
// Only the BORU between the delimiters is reformatted; the delimiter lines
// and every other byte of the document are preserved.
const (
	markerOpen  = "<!-- borufmt -->"
	markerClose = "<!-- /borufmt -->"
)

// FormatMarkdown reformats the BORU inside ```boru fenced code blocks and
// inside <!-- borufmt --> … <!-- /borufmt --> marker regions of a Markdown
// document, leaving the rest of the document unchanged.
func FormatMarkdown(src string) string {
	return formatMarkerRegions(formatFencedBlocks(src))
}

// FormatHTML reformats the BORU inside <!-- borufmt --> … <!-- /borufmt -->
// marker regions of an HTML document, leaving the rest unchanged.
func FormatHTML(src string) string {
	return formatMarkerRegions(src)
}

// formatDelimited reformats each region of src whose start line satisfies
// open (which returns a matcher for the region's closing line). The start
// and end lines are preserved verbatim; the body between them is passed
// through Format. A region left unclosed at end-of-input is still
// reformatted.
func formatDelimited(src string, open func(string) (func(string) bool, bool)) string {
	lines := strings.Split(src, "\n")
	var out []string
	i := 0
	for i < len(lines) {
		closes, ok := open(lines[i])
		if !ok {
			out = append(out, lines[i])
			i++
			continue
		}
		out = append(out, lines[i])
		j := i + 1
		var body []string
		for j < len(lines) && !closes(lines[j]) {
			body = append(body, lines[j])
			j++
		}
		out = append(out, reformatBody(body)...)
		if j < len(lines) {
			out = append(out, lines[j])
			j++
		}
		i = j
	}
	return strings.Join(out, "\n")
}

// reformatBody formats a collected block of BORU lines, returning the
// formatted lines with the trailing newline trimmed; an all-blank block
// yields no lines.
func reformatBody(body []string) []string {
	formatted := strings.TrimSuffix(Format(strings.Join(body, "\n")), "\n")
	if formatted == "" {
		return nil
	}
	return strings.Split(formatted, "\n")
}

// formatFencedBlocks reformats each fenced code block whose info string's
// first word is "boru".
func formatFencedBlocks(src string) string {
	return formatDelimited(src, func(line string) (func(string) bool, bool) {
		fence, ok := boruFenceOpen(line)
		if !ok {
			return nil, false
		}
		return func(l string) bool { return isFenceClose(l, fence) }, true
	})
}

// formatMarkerRegions reformats each <!-- borufmt --> … <!-- /borufmt -->
// region.
func formatMarkerRegions(src string) string {
	return formatDelimited(src, func(line string) (func(string) bool, bool) {
		if strings.TrimSpace(line) != markerOpen {
			return nil, false
		}
		return func(l string) bool { return strings.TrimSpace(l) == markerClose }, true
	})
}

// boruFenceOpen reports whether line opens a fenced code block whose info
// string's first word is "boru" (case-insensitive), returning the fence
// run (``` or ~~~) a matching close must repeat. The fence must start at
// column 0 so indented content is never reflowed out of place.
func boruFenceOpen(line string) (string, bool) {
	fence := leadingFenceRun(line)
	if fence == "" {
		return "", false
	}
	info := strings.Fields(line[len(fence):])
	if len(info) == 0 || !strings.EqualFold(info[0], "boru") {
		return "", false
	}
	return fence, true
}

// isFenceClose reports whether line closes an open fence: a run of the
// same character, at least as long, with nothing but spaces after it.
func isFenceClose(line, fence string) bool {
	run := leadingFenceRun(line)
	return run != "" && run[0] == fence[0] && len(run) >= len(fence) &&
		strings.TrimSpace(line[len(run):]) == ""
}

// leadingFenceRun returns the leading run of fence characters (``` or
// ~~~, length ≥ 3) at the very start of line, or "" if line does not
// begin with one.
func leadingFenceRun(line string) string {
	if line == "" || (line[0] != '`' && line[0] != '~') {
		return ""
	}
	ch := line[0]
	n := 0
	for n < len(line) && line[n] == ch {
		n++
	}
	if n < 3 {
		return ""
	}
	return line[:n]
}
