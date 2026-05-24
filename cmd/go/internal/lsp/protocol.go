// LSP protocol types used by the aql language server. Only the
// fields we actually read or write are modelled — JSON marshaling
// happily ignores extras the client sends.
//
// Reference: https://microsoft.github.io/language-server-protocol/
// specifications/lsp/3.17/specification/

package lsp

// Position is a zero-based (line, character) inside a document.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range covers (start, end) positions inclusive of start, exclusive
// of end (the LSP convention).
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// TextDocumentIdentifier identifies a document by URI.
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// VersionedTextDocumentIdentifier is the change-tracked variant.
type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

// TextDocumentItem is the full document state sent in didOpen.
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// DidOpenTextDocumentParams is the payload for textDocument/didOpen.
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// TextDocumentContentChangeEvent represents a single change. With
// full-sync (the only mode we advertise), Range is nil and Text is
// the entire new buffer.
type TextDocumentContentChangeEvent struct {
	Range *Range `json:"range,omitempty"`
	Text  string `json:"text"`
}

// DidChangeTextDocumentParams is the payload for textDocument/didChange.
type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

// DidCloseTextDocumentParams is the payload for textDocument/didClose.
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// HoverParams is the payload for textDocument/hover.
type HoverParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// CompletionParams is the payload for textDocument/completion.
type CompletionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// DocumentFormattingParams is the payload for textDocument/formatting.
type DocumentFormattingParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// DiagnosticSeverity values (1=Error, 2=Warning, 3=Information, 4=Hint).
const (
	severityError       = 1
	severityWarning     = 2
	severityInformation = 3
)

// Diagnostic is one finding reported via publishDiagnostics.
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity,omitempty"`
	Code     string `json:"code,omitempty"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}

// PublishDiagnosticsParams is the payload for textDocument/publishDiagnostics.
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// MarkupContent is the rich text shape used by hover. We always
// emit Kind="plaintext"; switching to markdown later is one
// constant change.
type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// Hover is the response to textDocument/hover.
type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

// CompletionItemKind values used by the spec. We only need Function (3).
const (
	completionKindFunction = 3
)

// CompletionItem is one entry in a completion response.
type CompletionItem struct {
	Label         string `json:"label"`
	Detail        string `json:"detail,omitempty"`
	Documentation string `json:"documentation,omitempty"`
	Kind          int    `json:"kind,omitempty"`
	InsertText    string `json:"insertText,omitempty"`
}

// TextEdit is a single edit produced by formatting.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// TextDocumentSyncKind values used in ServerCapabilities.
const (
	syncFull = 1
)

// CompletionOptions is the completion provider capability struct.
type CompletionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
}

// ServerCapabilities advertises what the server supports.
type ServerCapabilities struct {
	TextDocumentSync           int                `json:"textDocumentSync"`
	HoverProvider              bool               `json:"hoverProvider"`
	CompletionProvider         *CompletionOptions `json:"completionProvider,omitempty"`
	DocumentFormattingProvider bool               `json:"documentFormattingProvider"`
}

// InitializeResult is the response to the initialize request.
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   *ServerInfo        `json:"serverInfo,omitempty"`
}

// ServerInfo is optional metadata returned with initialize.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
