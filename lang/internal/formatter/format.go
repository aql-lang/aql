package formatter

import (
	"strings"
	"unicode"
)

const maxLineWidth = 70

// knownTypes lists type names that should be capitalised
// when they appear in type annotation positions (after :).
var knownTypes = map[string]bool{
	"any": true, "none": true, "scalar": true, "number": true,
	"integer": true, "decimal": true, "string": true, "boolean": true,
	"atom": true, "node": true, "list": true, "map": true,
	"table": true, "record": true, "object": true, "function": true,
}

// TokenKind classifies a token in the AQL source.
type TokenKind int

const (
	TokWord TokenKind = iota
	TokString
	TokNumber
	TokComment
	TokBlockComment
	TokLBracket
	TokRBracket
	TokLBrace
	TokRBrace
	TokLParen
	TokRParen
	TokComma
	TokColon
	TokSemicolon
	TokDot
	TokQuestion
	TokBang
	TokPipe
	TokNewline
)

// Token is a lexical token from AQL source.
type Token struct {
	Kind TokenKind
	Text string
}

// NodeKind classifies a node in the format tree.
type NodeKind int

const (
	NdRoot NodeKind = iota
	NdWord
	NdString
	NdNumber
	NdComment
	NdBlockComment
	NdList
	NdMap
	NdParen
	NdComma
	NdColon
	NdSemicolon
	NdDot
	NdQuestion
	NdBang
	NdPipe
	NdNewline
)

// Node is a node in the format tree.
type Node struct {
	Kind     NodeKind
	Text     string
	Children []*Node
}

// Format formats AQL source code.
func Format(src string) string {
	tokens := tokenize(src)
	tree := buildTree(tokens)
	capitalizeTypesInTree(tree)
	return emitRoot(tree, 0)
}

// --- Tokenizer ---

func tokenize(src string) []Token {
	var tokens []Token
	i := 0
	for i < len(src) {
		ch := src[i]

		if ch == ' ' || ch == '\t' {
			i++
			continue
		}

		if ch == '\n' {
			tokens = append(tokens, Token{TokNewline, "\n"})
			i++
			continue
		}
		if ch == '\r' {
			i++
			if i < len(src) && src[i] == '\n' {
				i++
			}
			tokens = append(tokens, Token{TokNewline, "\n"})
			continue
		}

		// Block comment ## ... ##
		if ch == '#' && i+1 < len(src) && src[i+1] == '#' {
			end := strings.Index(src[i+2:], "##")
			if end < 0 {
				tokens = append(tokens, Token{TokBlockComment, src[i:]})
				i = len(src)
			} else {
				tokens = append(tokens, Token{TokBlockComment, src[i : i+2+end+2]})
				i = i + 2 + end + 2
			}
			continue
		}

		// Line comment
		if ch == '#' {
			end := strings.IndexByte(src[i:], '\n')
			if end < 0 {
				tokens = append(tokens, Token{TokComment, src[i:]})
				i = len(src)
			} else {
				tokens = append(tokens, Token{TokComment, src[i : i+end]})
				i = i + end
			}
			continue
		}

		// Strings
		if ch == '"' || ch == '\'' {
			s, n := scanString(src[i:])
			tokens = append(tokens, Token{TokString, s})
			i += n
			continue
		}

		// Single-char tokens
		if kind, ok := singleCharToken(ch); ok {
			tokens = append(tokens, Token{kind, string(ch)})
			i++
			continue
		}

		// Dot
		if ch == '.' {
			if len(tokens) > 0 && tokens[len(tokens)-1].Kind == TokNumber &&
				i+1 < len(src) && src[i+1] >= '0' && src[i+1] <= '9' {
				j := i + 1
				for j < len(src) && src[j] >= '0' && src[j] <= '9' {
					j++
				}
				tokens[len(tokens)-1].Text += src[i:j]
				i = j
				continue
			}
			tokens = append(tokens, Token{TokDot, "."})
			i++
			continue
		}

		// Numbers
		if ch >= '0' && ch <= '9' {
			tokens = append(tokens, Token{TokNumber, scanNumber(src, i)})
			i += len(tokens[len(tokens)-1].Text)
			continue
		}
		if ch == '-' && i+1 < len(src) && src[i+1] >= '0' && src[i+1] <= '9' {
			tokens = append(tokens, Token{TokNumber, scanNumber(src, i)})
			i += len(tokens[len(tokens)-1].Text)
			continue
		}

		// Words (including dotted words like foo.bar)
		j := i
		for j < len(src) && !isDelimiter(src[j]) {
			j++
		}
		if j > i {
			tokens = append(tokens, Token{TokWord, src[i:j]})
			i = j
			continue
		}

		i++
	}
	return tokens
}

func singleCharToken(ch byte) (TokenKind, bool) {
	switch ch {
	case '[':
		return TokLBracket, true
	case ']':
		return TokRBracket, true
	case '{':
		return TokLBrace, true
	case '}':
		return TokRBrace, true
	case '(':
		return TokLParen, true
	case ')':
		return TokRParen, true
	case ',':
		return TokComma, true
	case ':':
		return TokColon, true
	case ';':
		return TokSemicolon, true
	case '?':
		return TokQuestion, true
	case '!':
		return TokBang, true
	case '|':
		return TokPipe, true
	}
	return 0, false
}

func scanNumber(src string, start int) string {
	j := start
	if j < len(src) && src[j] == '-' {
		j++
	}
	for j < len(src) && src[j] >= '0' && src[j] <= '9' {
		j++
	}
	if j < len(src) && src[j] == '.' && j+1 < len(src) && src[j+1] >= '0' && src[j+1] <= '9' {
		j++
		for j < len(src) && src[j] >= '0' && src[j] <= '9' {
			j++
		}
	}
	return src[start:j]
}

func isDelimiter(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r',
		'[', ']', '{', '}', '(', ')',
		',', ';', ':', '?', '!', '|', '#':
		return true
	}
	return false
}

func scanString(s string) (string, int) {
	if len(s) < 1 {
		return "", 0
	}
	quote := s[0]
	i := 1
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			i += 2
			continue
		}
		if s[i] == quote {
			return s[:i+1], i + 1
		}
		i++
	}
	return s, len(s)
}

// --- Tree builder ---

func buildTree(tokens []Token) *Node {
	root := &Node{Kind: NdRoot}
	stack := []*Node{root}
	cur := func() *Node { return stack[len(stack)-1] }
	push := func(n *Node) {
		cur().Children = append(cur().Children, n)
		stack = append(stack, n)
	}
	pop := func() {
		if len(stack) > 1 {
			stack = stack[:len(stack)-1]
		}
	}
	add := func(n *Node) {
		cur().Children = append(cur().Children, n)
	}

	for _, tok := range tokens {
		switch tok.Kind {
		case TokLBracket:
			push(&Node{Kind: NdList})
		case TokRBracket:
			for len(stack) > 1 && cur().Kind != NdList {
				pop()
			}
			pop()
		case TokLBrace:
			push(&Node{Kind: NdMap})
		case TokRBrace:
			for len(stack) > 1 && cur().Kind != NdMap {
				pop()
			}
			pop()
		case TokLParen:
			push(&Node{Kind: NdParen})
		case TokRParen:
			for len(stack) > 1 && cur().Kind != NdParen {
				pop()
			}
			pop()
		case TokWord:
			add(&Node{Kind: NdWord, Text: tok.Text})
		case TokString:
			add(&Node{Kind: NdString, Text: tok.Text})
		case TokNumber:
			add(&Node{Kind: NdNumber, Text: tok.Text})
		case TokComment:
			add(&Node{Kind: NdComment, Text: tok.Text})
		case TokBlockComment:
			add(&Node{Kind: NdBlockComment, Text: tok.Text})
		case TokNewline:
			add(&Node{Kind: NdNewline})
		default:
			kind := tokenToNodeKind(tok.Kind)
			add(&Node{Kind: kind, Text: tok.Text})
		}
	}
	return root
}

func tokenToNodeKind(tk TokenKind) NodeKind {
	switch tk {
	case TokComma:
		return NdComma
	case TokColon:
		return NdColon
	case TokSemicolon:
		return NdSemicolon
	case TokDot:
		return NdDot
	case TokQuestion:
		return NdQuestion
	case TokBang:
		return NdBang
	case TokPipe:
		return NdPipe
	}
	return NdWord
}

// --- Type capitalization ---

// capitalizeTypesInTree walks the tree and capitalizes known type
// names. A word is capitalized if it's a known type name AND:
//   - it appears after : (type annotation), OR
//   - it appears as a standalone word (not after def, not before :)
//
// Words are NOT capitalized when:
//   - immediately after "def" (variable name being defined)
//   - immediately before ":" (parameter/key name)
//   - they contain a "." (dotted word like table.kind)
func capitalizeTypesInTree(n *Node) {
	for i, ch := range n.Children {
		if ch.Kind == NdWord && !strings.Contains(ch.Text, ".") {
			lower := strings.ToLower(ch.Text)
			if knownTypes[lower] && !isKeyword(lower) {
				// Skip if after "def" or "type" (it's a name being defined).
				afterDef := i > 0 && n.Children[i-1].Kind == NdWord &&
					(n.Children[i-1].Text == "def" || n.Children[i-1].Text == "type")
				// Skip if after a word that itself follows "type" (e.g. type Cond record).
				afterTypeName := i > 1 && n.Children[i-2].Kind == NdWord &&
					(n.Children[i-2].Text == "type" || n.Children[i-2].Text == "def")
				// Skip if before ":" (it's a key/param name).
				beforeColon := i+1 < len(n.Children) && n.Children[i+1].Kind == NdColon
				if !afterDef && !afterTypeName && !beforeColon {
					ch.Text = capitalize(lower)
				}
			}
		}
		// Recurse into containers.
		if ch.Kind == NdList || ch.Kind == NdMap || ch.Kind == NdParen || ch.Kind == NdRoot {
			capitalizeTypesInTree(ch)
		}
	}
}

// isKeyword returns true for type names that are also used as
// AQL keywords and should not be auto-capitalized.
func isKeyword(lower string) bool {
	switch lower {
	case "record", "object", "function":
		return true
	}
	return false
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}



// --- Emitter ---

func emitRoot(n *Node, indent int) string {
	stmts := splitStatements(n.Children)
	var lines []string
	prevBlank := false
	for _, stmt := range stmts {
		if len(stmt) == 0 {
			if !prevBlank && len(lines) > 0 {
				lines = append(lines, "")
				prevBlank = true
			}
			continue
		}
		prevBlank = false
		lines = append(lines, emitStatement(stmt, indent))
	}
	result := strings.Join(lines, "\n")
	if result != "" && !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	return result
}

func splitStatements(children []*Node) [][]*Node {
	var stmts [][]*Node
	var cur []*Node
	for _, ch := range children {
		if ch.Kind == NdNewline {
			stmts = append(stmts, cur)
			cur = nil
			continue
		}
		cur = append(cur, ch)
	}
	if len(cur) > 0 {
		stmts = append(stmts, cur)
	}
	return stmts
}

func emitStatement(nodes []*Node, indent int) string {
	if len(nodes) == 0 {
		return ""
	}
	if nodes[0].Kind == NdComment || nodes[0].Kind == NdBlockComment {
		return strings.Repeat(" ", indent) + nodes[0].Text
	}

	line := strings.Repeat(" ", indent) + renderInline(nodes, indent)
	if len(line) <= maxLineWidth {
		return line
	}

	// Try fn-specific formatting first.
	if fn := tryFnFormat(nodes, indent); fn != "" {
		return fn
	}

	// If statement ends with a container (map/list), format it
	// with the opening bracket on the same line.
	if r := tryTrailingContainer(nodes, indent); r != "" {
		return r
	}

	return wrapStatement(nodes, indent)
}

// tryFnFormat handles def name fn [[args] [returns] [body]] formatting.
// The wrapper list contains three inner lists (one signature triple).
// Returns "" if not applicable.
func tryFnFormat(nodes []*Node, indent int) string {
	// Look for pattern: ... fn [wrapper]
	fnIdx := -1
	for i, n := range nodes {
		if n.Kind == NdWord && n.Text == "fn" {
			fnIdx = i
			break
		}
	}
	if fnIdx < 0 || fnIdx+1 >= len(nodes) {
		return ""
	}

	wrapper := nodes[fnIdx+1]
	if wrapper.Kind != NdList {
		return ""
	}

	// Extract the inner lists from the wrapper.
	var inner []*Node
	for _, ch := range wrapper.Children {
		if ch.Kind == NdList {
			inner = append(inner, ch)
		}
	}
	if len(inner) < 3 || len(inner)%3 != 0 {
		return ""
	}

	prefix := strings.Repeat(" ", indent)

	// Header: everything before fn + fn + [args] [returns]
	headerParts := nodes[:fnIdx+1]
	headerStr := renderInline(headerParts, indent)
	argsStr := emitNode(inner[0], indent)
	retStr := emitNode(inner[1], indent)
	header := prefix + headerStr + " [" + argsStr + " " + retStr

	body := inner[2]
	bodyChildren := nonTrivial(body.Children)
	bodyInline := renderInline(bodyChildren, indent)

	// Try single line: def name fn [[args] [returns] [body]]
	oneLine := header + " [" + bodyInline + "]]"
	if len(oneLine) <= maxLineWidth {
		return oneLine
	}

	// If header + " [" is too long, wrap it.
	if len(header)+2 > maxLineWidth {
		header = wrapStatement(append(headerParts,
			&Node{Kind: NdWord, Text: "[" + argsStr},
			&Node{Kind: NdWord, Text: retStr},
		), indent)
	}

	// Multi-line: header [
	//   body
	// ]]
	bodyIndent := indent + 2
	groups := splitIntoGroups(bodyChildren)
	var lines []string
	lines = append(lines, header+" [")
	for _, grp := range groups {
		grpLine := strings.Repeat(" ", bodyIndent) + renderInline(grp, bodyIndent)
		if len(grpLine) <= maxLineWidth {
			lines = append(lines, grpLine)
		} else {
			lines = append(lines, wrapStatement(grp, bodyIndent))
		}
	}
	lines = append(lines, prefix+"]]")

	return strings.Join(lines, "\n")
}

// renderInline renders nodes on a single line with proper attachment.
func renderInline(nodes []*Node, indent int) string {
	var parts []string
	for i, n := range nodes {
		s := emitNode(n, indent)
		if attach(nodes, i) && len(parts) > 0 {
			parts[len(parts)-1] += s
			continue
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

// attach returns true if node at index i should be attached to
// the previous token (no space before it).
func attach(nodes []*Node, i int) bool {
	n := nodes[i]
	// Comma, colon, question, dot attach to previous.
	if n.Kind == NdComma || n.Kind == NdColon || n.Kind == NdQuestion || n.Kind == NdDot {
		return true
	}
	// After colon or dot, attach to it.
	if i > 0 && (nodes[i-1].Kind == NdColon || nodes[i-1].Kind == NdDot) {
		return true
	}
	// If previous word ends with '.', attach (e.g., input.( → no space).
	if i > 0 && nodes[i-1].Kind == NdWord && strings.HasSuffix(nodes[i-1].Text, ".") {
		return true
	}
	return false
}

func emitNode(n *Node, indent int) string {
	switch n.Kind {
	case NdRoot:
		return emitRoot(n, indent)
	case NdList:
		return emitList(n, indent)
	case NdMap:
		return emitMap(n, indent)
	case NdParen:
		return emitParen(n, indent)
	case NdWord, NdNumber, NdString:
		return n.Text
	case NdComment, NdBlockComment:
		return n.Text
	case NdComma:
		return ","
	case NdColon:
		return ":"
	case NdSemicolon:
		return ";"
	case NdDot:
		return "."
	case NdQuestion:
		return "?"
	case NdBang:
		return "!"
	case NdPipe:
		return "|"
	case NdNewline:
		return ""
	}
	return ""
}

// tryTrailingContainer handles statements ending with a map or list.
// Keeps the opening bracket on the same line as preceding tokens.
func tryTrailingContainer(nodes []*Node, indent int) string {
	if len(nodes) < 2 {
		return ""
	}
	last := nodes[len(nodes)-1]
	if last.Kind != NdMap && last.Kind != NdList {
		return ""
	}

	prefix := strings.Repeat(" ", indent)
	head := nodes[:len(nodes)-1]
	headStr := renderInline(head, indent)
	container := last

	if container.Kind == NdMap {
		children := nonTrivial(container.Children)
		entries := parseMapEntries(children)

		// Try single line.
		single := prefix + headStr + " {" + renderMapEntries(entries, indent) + "}"
		if len(single) <= maxLineWidth {
			return single
		}

		// Multi-line with { on header line.
		childIndent := indent + 2
		childPfx := strings.Repeat(" ", childIndent)
		var lines []string
		lines = append(lines, prefix+headStr+" {")
		for _, e := range entries {
			lines = append(lines, childPfx+renderMapEntry(e, childIndent))
		}
		lines = append(lines, prefix+"}")
		return strings.Join(lines, "\n")
	}

	if container.Kind == NdList {
		children := nonTrivial(container.Children)
		inner := renderInline(children, indent)
		single := prefix + headStr + " [" + inner + "]"
		if len(single) <= maxLineWidth {
			return single
		}

		// Multi-line with [ on header line.
		groups := splitIntoGroups(children)
		bodyIndent := indent + 2
		var lines []string
		lines = append(lines, prefix+headStr+" [")
		for _, grp := range groups {
			grpLine := strings.Repeat(" ", bodyIndent) + renderInline(grp, bodyIndent)
			if len(grpLine) <= maxLineWidth {
				lines = append(lines, grpLine)
			} else {
				lines = append(lines, wrapStatement(grp, bodyIndent))
			}
		}
		lines = append(lines, prefix+"]")
		return strings.Join(lines, "\n")
	}

	return ""
}

// wrapStatement breaks a long statement across multiple lines.
// It tries to break before container nodes (lists, maps, parens)
// and keeps logical units together.
func wrapStatement(nodes []*Node, indent int) string {
	prefix := strings.Repeat(" ", indent)
	contPrefix := strings.Repeat(" ", indent+2)

	var lines []string
	var cur []string
	curLen := indent

	flush := func(nextPrefix string) {
		if len(cur) > 0 {
			lines = append(lines, prefix+strings.Join(cur, " "))
			cur = nil
			curLen = len(nextPrefix)
			prefix = nextPrefix
		}
	}

	for i, n := range nodes {
		s := emitNode(n, indent+2)
		if attach(nodes, i) && len(cur) > 0 {
			cur[len(cur)-1] += s
			curLen += len(s)
			continue
		}

		tokenLen := len(s) + 1
		if curLen+tokenLen > maxLineWidth && len(cur) > 0 {
			flush(contPrefix)
		}
		cur = append(cur, s)
		curLen += tokenLen
	}
	flush(contPrefix)
	return strings.Join(lines, "\n")
}

// emitList formats [...].
func emitList(n *Node, indent int) string {
	children := nonTrivial(n.Children)
	if len(children) == 0 {
		return "[]"
	}

	// Try single line.
	inner := renderInline(children, indent)
	single := "[" + inner + "]"
	if len(single)+indent <= maxLineWidth {
		return single
	}

	// Multi-line. Split children into logical groups for wrapping.
	groups := splitIntoGroups(children)
	childIndent := indent + 2
	childPfx := strings.Repeat(" ", childIndent)

	var lines []string
	for gi, grp := range groups {
		line := renderInline(grp, childIndent)
		full := childPfx + line
		if len(full) <= maxLineWidth {
			if gi == 0 {
				lines = append(lines, "["+line)
			} else {
				lines = append(lines, full)
			}
		} else {
			wrapped := wrapStatement(grp, childIndent)
			if gi == 0 {
				// Put [ on first line of wrapped.
				wlines := strings.Split(wrapped, "\n")
				wlines[0] = "[" + strings.TrimLeft(wlines[0], " ")
				lines = append(lines, strings.Join(wlines, "\n"))
			} else {
				lines = append(lines, wrapped)
			}
		}
	}
	lines = append(lines, strings.Repeat(" ", indent)+"]")
	return strings.Join(lines, "\n")
}

// splitIntoGroups splits a list's children into logical statement
// groups. A new group starts at statement-start words:
// def, type, if, for, export, end.
func splitIntoGroups(children []*Node) [][]*Node {
	var groups [][]*Node
	var cur []*Node

	for _, ch := range children {
		if ch.Kind == NdWord && isStmtStart(ch.Text) && len(cur) > 0 {
			groups = append(groups, cur)
			cur = nil
		}
		cur = append(cur, ch)
	}
	if len(cur) > 0 {
		groups = append(groups, cur)
	}
	if len(groups) == 0 {
		return [][]*Node{children}
	}
	return groups
}

func isStmtStart(word string) bool {
	switch word {
	case "def", "type", "if", "for", "export", "end", "make":
		return true
	}
	return false
}

// emitMap formats {...}.
func emitMap(n *Node, indent int) string {
	children := nonTrivial(n.Children)
	if len(children) == 0 {
		return "{}"
	}

	// Parse key:value entries.
	entries := parseMapEntries(children)

	// Try single line.
	single := "{" + renderMapEntries(entries, indent) + "}"
	if len(single)+indent <= maxLineWidth {
		return single
	}

	// Multi-line: first entry on { line, rest indented.
	childIndent := indent + 2
	childPfx := strings.Repeat(" ", childIndent)
	var lines []string
	first := renderMapEntry(entries[0], childIndent)
	lines = append(lines, "{"+first)
	for _, e := range entries[1:] {
		lines = append(lines, childPfx+renderMapEntry(e, childIndent))
	}
	lines = append(lines, strings.Repeat(" ", indent)+"}")
	return strings.Join(lines, "\n")
}

type mapEntry struct {
	key      string
	optional bool
	value    *Node
	comment  *Node // standalone comment
}

func parseMapEntries(children []*Node) []mapEntry {
	var entries []mapEntry
	i := 0
	for i < len(children) {
		ch := children[i]
		if ch.Kind == NdComma {
			i++
			continue
		}
		if ch.Kind == NdComment || ch.Kind == NdBlockComment {
			entries = append(entries, mapEntry{comment: ch})
			i++
			continue
		}
		// key ? : value
		if i+3 < len(children) &&
			children[i+1].Kind == NdQuestion &&
			children[i+2].Kind == NdColon {
			entries = append(entries, mapEntry{
				key:      ch.Text,
				optional: true,
				value:    children[i+3],
			})
			i += 4
			continue
		}
		// key : value
		if i+2 < len(children) && children[i+1].Kind == NdColon {
			entries = append(entries, mapEntry{
				key:   ch.Text,
				value: children[i+2],
			})
			i += 3
			continue
		}
		// Bare value (typed map child syntax).
		entries = append(entries, mapEntry{
			value: ch,
		})
		i++
	}
	return entries
}

func renderMapEntries(entries []mapEntry, indent int) string {
	var parts []string
	for _, e := range entries {
		parts = append(parts, renderMapEntry(e, indent))
	}
	return strings.Join(parts, " ")
}

func renderMapEntry(e mapEntry, indent int) string {
	if e.comment != nil {
		return e.comment.Text
	}
	if e.key == "" {
		return emitNode(e.value, indent)
	}
	opt := ""
	if e.optional {
		opt = "?"
	}
	return e.key + opt + ":" + emitNode(e.value, indent)
}

// emitParen formats (...).
func emitParen(n *Node, indent int) string {
	children := nonTrivial(n.Children)
	if len(children) == 0 {
		return "()"
	}

	inner := renderInline(children, indent)
	single := "(" + inner + ")"
	if len(single)+indent <= maxLineWidth {
		return single
	}

	// Multi-line.
	childIndent := indent + 2
	inner = renderInline(children, childIndent)
	full := "(" + inner + ")"
	if len(full)+indent <= maxLineWidth {
		return full
	}

	var lines []string
	wrapped := wrapStatement(children, childIndent)
	wlines := strings.Split(wrapped, "\n")
	wlines[0] = "(" + strings.TrimLeft(wlines[0], " ")
	lines = append(lines, wlines...)
	lines = append(lines, strings.Repeat(" ", indent)+")")
	return strings.Join(lines, "\n")
}

// nonTrivial filters out newlines and commas.
func nonTrivial(nodes []*Node) []*Node {
	var out []*Node
	for _, n := range nodes {
		if n.Kind != NdNewline {
			out = append(out, n)
		}
	}
	return out
}
