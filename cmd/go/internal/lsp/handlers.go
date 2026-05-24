// LSP method dispatch and server lifecycle.
//
// The server runs a single-threaded read-dispatch loop: every
// message is decoded, routed to the relevant handler, and the
// handler either responds synchronously (requests) or fires a
// notification on its way out (didOpen/didChange both publish
// diagnostics). Concurrency comes only from conn.notify being
// safe to call from anywhere — the dispatch loop itself does not
// fork goroutines.

package lsp

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/aql-lang/aql/lang/go/native"
)

// server holds the per-connection state.
type server struct {
	conn      *conn
	buffers   map[string]string // URI → buffer text
	versions  map[string]int    // URI → last version (for didChange filtering)
	registry  *native.Registry  // built once on demand, used by hover
	shutdown  bool              // set by shutdown request; controls exit code
	stderrLog io.Writer         // for logging unexpected errors
}

// newServer constructs a server connected to r/w. stderr receives
// log lines (mostly decode-error notes); it is not the LSP log
// channel (that would go via window/logMessage if we needed it).
func newServer(r io.Reader, w io.Writer, stderr io.Writer) *server {
	return &server{
		conn:      newConn(r, w),
		buffers:   make(map[string]string),
		versions:  make(map[string]int),
		stderrLog: stderr,
	}
}

// run is the dispatch loop. It returns when the underlying reader
// closes (io.EOF) or an unrecoverable transport error occurs.
// Method handlers signal an "exit cleanly" via setting s.shutdown
// before the client sends "exit"; the function returns 0 if the
// shutdown handshake was followed, 1 otherwise (LSP convention).
func (s *server) run() int {
	for {
		raw, err := s.conn.readMessage()
		if err != nil {
			if err == io.EOF {
				if s.shutdown {
					return 0
				}
				return 1
			}
			fmt.Fprintf(s.stderrLog, "lsp: read error: %s\n", err)
			return 1
		}

		var msg rawMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			fmt.Fprintf(s.stderrLog, "lsp: parse error: %s\n", err)
			continue
		}

		if msg.Method == "exit" {
			if s.shutdown {
				return 0
			}
			return 1
		}

		s.dispatch(&msg)
	}
}

// dispatch routes one message to the correct handler. Errors during
// handling are reported either as a JSON-RPC error response (for
// requests) or logged to stderr (for notifications).
func (s *server) dispatch(msg *rawMessage) {
	switch msg.Method {
	case "initialize":
		s.handleInitialize(msg)
	case "initialized":
		// no-op
	case "shutdown":
		s.shutdown = true
		if msg.hasID() {
			// If the write fails the next readMessage returns EOF
			// and the dispatch loop terminates naturally.
			_ = s.conn.respond(msg.ID, nil)
		}
	case "textDocument/didOpen":
		s.handleDidOpen(msg)
	case "textDocument/didChange":
		s.handleDidChange(msg)
	case "textDocument/didClose":
		s.handleDidClose(msg)
	case "textDocument/hover":
		s.handleHover(msg)
	case "textDocument/completion":
		s.handleCompletion(msg)
	case "textDocument/formatting":
		s.handleFormatting(msg)
	default:
		if msg.hasID() {
			_ = s.conn.respondError(msg.ID, errMethodNotFound, "method not found: "+msg.Method)
		}
	}
}

// handleInitialize replies with the server capabilities. We
// advertise full-document sync, hover, completion (no trigger
// characters — client requests on demand), and formatting.
func (s *server) handleInitialize(msg *rawMessage) {
	if !msg.hasID() {
		return
	}
	result := InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync:           syncFull,
			HoverProvider:              true,
			CompletionProvider:         &CompletionOptions{},
			DocumentFormattingProvider: true,
		},
		ServerInfo: &ServerInfo{
			Name:    "aql-lsp",
			Version: "0.1.0",
		},
	}
	_ = s.conn.respond(msg.ID, result)
}

// handleDidOpen caches the buffer and publishes diagnostics.
func (s *server) handleDidOpen(msg *rawMessage) {
	var p DidOpenTextDocumentParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		fmt.Fprintf(s.stderrLog, "lsp: didOpen decode: %s\n", err)
		return
	}
	s.buffers[p.TextDocument.URI] = p.TextDocument.Text
	s.versions[p.TextDocument.URI] = p.TextDocument.Version
	s.publishDiagnostics(p.TextDocument.URI)
}

// handleDidChange updates the buffer (full-sync only) and
// publishes fresh diagnostics. Multiple ContentChanges arriving in
// a single message are applied in order; with full-sync the last
// entry's Text is the final buffer.
func (s *server) handleDidChange(msg *rawMessage) {
	var p DidChangeTextDocumentParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		fmt.Fprintf(s.stderrLog, "lsp: didChange decode: %s\n", err)
		return
	}
	uri := p.TextDocument.URI
	for _, ch := range p.ContentChanges {
		// Full-sync only: Range is nil. If a client sends ranged
		// changes despite our capability declaration, treat each
		// as a full replacement (their last change wins).
		s.buffers[uri] = ch.Text
	}
	s.versions[uri] = p.TextDocument.Version
	s.publishDiagnostics(uri)
}

// handleDidClose drops the buffer and clears any published
// diagnostics so the editor's gutter doesn't keep stale squigglies.
func (s *server) handleDidClose(msg *rawMessage) {
	var p DidCloseTextDocumentParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		fmt.Fprintf(s.stderrLog, "lsp: didClose decode: %s\n", err)
		return
	}
	delete(s.buffers, p.TextDocument.URI)
	delete(s.versions, p.TextDocument.URI)
	_ = s.conn.notify("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI:         p.TextDocument.URI,
		Diagnostics: []Diagnostic{},
	})
}

// handleHover responds with help for the word at the cursor, or
// null if no word is there.
func (s *server) handleHover(msg *rawMessage) {
	if !msg.hasID() {
		return
	}
	var p HoverParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.conn.respondError(msg.ID, errInvalidRequest, "invalid hover params")
		return
	}
	src, ok := s.buffers[p.TextDocument.URI]
	if !ok {
		_ = s.conn.respond(msg.ID, nil)
		return
	}
	hover := s.buildHover(src, p.Position)
	if hover == nil {
		_ = s.conn.respond(msg.ID, nil)
		return
	}
	_ = s.conn.respond(msg.ID, hover)
}

// handleCompletion responds with the static list of known words.
func (s *server) handleCompletion(msg *rawMessage) {
	if !msg.hasID() {
		return
	}
	items := s.buildCompletionItems()
	_ = s.conn.respond(msg.ID, items)
}

// handleFormatting reformats the buffer and returns a single
// TextEdit replacing the whole document.
func (s *server) handleFormatting(msg *rawMessage) {
	if !msg.hasID() {
		return
	}
	var p DocumentFormattingParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.conn.respondError(msg.ID, errInvalidRequest, "invalid formatting params")
		return
	}
	src, ok := s.buffers[p.TextDocument.URI]
	if !ok {
		_ = s.conn.respond(msg.ID, []TextEdit{})
		return
	}
	edits := s.buildFormattingEdits(src)
	_ = s.conn.respond(msg.ID, edits)
}

// ensureRegistry lazily builds the native registry so hover/completion
// can resolve words. Failure to build is non-fatal; we just return nil.
func (s *server) ensureRegistry() *native.Registry {
	if s.registry != nil {
		return s.registry
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		fmt.Fprintf(s.stderrLog, "lsp: native registry: %s\n", err)
		return nil
	}
	s.registry = reg
	return reg
}
