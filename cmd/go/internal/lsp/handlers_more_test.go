// Direct dispatch-loop tests: each test pre-frames a full message
// sequence, runs the server synchronously against it, and inspects
// the framed responses it wrote. No goroutines or pipes — the loop
// is single-threaded, so the output order is deterministic.

package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go/formatter"
)

// runServer feeds the framed input to a fresh server and returns the
// exit code plus the raw stdout/stderr buffers.
func runServer(t *testing.T, input []byte) (int, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, stderr bytes.Buffer
	s := newServer(bytes.NewReader(input), &out, &stderr)
	code := s.run()
	return code, &out, &stderr
}

// parseServerOutput splits the server's framed output back into
// individual JSON-RPC messages, in write order.
func parseServerOutput(t *testing.T, out *bytes.Buffer) []rawMessage {
	t.Helper()
	br := bufio.NewReader(bytes.NewReader(out.Bytes()))
	var msgs []rawMessage
	for {
		data, err := readFramedMessage(br)
		if err != nil {
			return msgs
		}
		var m rawMessage
		if jerr := json.Unmarshal(data, &m); jerr != nil {
			t.Fatalf("bad frame %q: %s", data, jerr)
		}
		msgs = append(msgs, m)
	}
}

// responseFor finds the response with the given integer id, or nil.
func responseFor(id int, msgs []rawMessage) *rawMessage {
	for i := range msgs {
		if !msgs[i].hasID() {
			continue
		}
		var got int
		if json.Unmarshal(msgs[i].ID, &got) == nil && got == id {
			return &msgs[i]
		}
	}
	return nil
}

// diagnosticsNotifications extracts every publishDiagnostics payload,
// in receipt order.
func diagnosticsNotifications(t *testing.T, msgs []rawMessage) []PublishDiagnosticsParams {
	t.Helper()
	var out []PublishDiagnosticsParams
	for i := range msgs {
		if msgs[i].hasID() || msgs[i].Method != "textDocument/publishDiagnostics" {
			continue
		}
		var pd PublishDiagnosticsParams
		if err := json.Unmarshal(msgs[i].Params, &pd); err != nil {
			t.Fatalf("bad publishDiagnostics params: %s", err)
		}
		out = append(out, pd)
	}
	return out
}

// didOpenFrame frames a textDocument/didOpen notification.
func didOpenFrame(t *testing.T, uri, text string) []byte {
	t.Helper()
	return frameBytes(t, rpcNotification{
		JSONRPC: "2.0", Method: "textDocument/didOpen",
		Params: DidOpenTextDocumentParams{
			TextDocument: TextDocumentItem{URI: uri, LanguageID: "aql", Version: 1, Text: text},
		},
	})
}

// shutdownExitFrames frames the clean-termination tail.
func shutdownExitFrames(t *testing.T, id int) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.Write(frameBytes(t, rpcRequest{JSONRPC: "2.0", ID: id, Method: "shutdown"}))
	buf.Write(frameBytes(t, rpcNotification{JSONRPC: "2.0", Method: "exit"}))
	return buf.Bytes()
}

// --- run loop ---

func TestRunExitWithoutShutdownReturns1(t *testing.T) {
	code, _, _ := runServer(t, frameBytes(t, rpcNotification{JSONRPC: "2.0", Method: "exit"}))
	if code != 1 {
		t.Errorf("run() = %d, want 1 for exit without shutdown", code)
	}
}

func TestRunEOFAfterShutdownReturns0(t *testing.T) {
	// shutdown followed by EOF (no explicit exit) still counts as a
	// followed handshake.
	code, _, _ := runServer(t, frameBytes(t, rpcRequest{JSONRPC: "2.0", ID: 1, Method: "shutdown"}))
	if code != 0 {
		t.Errorf("run() = %d, want 0 for EOF after shutdown", code)
	}
}

func TestRunShutdownNotificationThenExit(t *testing.T) {
	// shutdown sent without an id (as a notification) must not produce
	// a response but still arms the clean exit.
	var input bytes.Buffer
	input.Write(frameBytes(t, rpcNotification{JSONRPC: "2.0", Method: "shutdown"}))
	input.Write(frameBytes(t, rpcNotification{JSONRPC: "2.0", Method: "exit"}))

	code, out, _ := runServer(t, input.Bytes())
	if code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
	if msgs := parseServerOutput(t, out); len(msgs) != 0 {
		t.Errorf("expected no responses for notification-only session, got %d", len(msgs))
	}
}

func TestRunMalformedJSONLogsAndContinues(t *testing.T) {
	var input bytes.Buffer
	garbage := []byte("{not json")
	input.WriteString(fmt.Sprintf("Content-Length: %d\r\n\r\n", len(garbage)))
	input.Write(garbage)
	input.Write(shutdownExitFrames(t, 1))

	code, _, stderr := runServer(t, input.Bytes())
	if code != 0 {
		t.Errorf("run() = %d, want 0 (loop must survive one bad frame)", code)
	}
	if !strings.Contains(stderr.String(), "parse error") {
		t.Errorf("stderr = %q, want a parse error note", stderr.String())
	}
}

func TestRunTransportErrorLogsAndReturns1(t *testing.T) {
	code, _, stderr := runServer(t, []byte("Content-Length: notanumber\r\n\r\n"))
	if code != 1 {
		t.Errorf("run() = %d, want 1 for transport error", code)
	}
	if !strings.Contains(stderr.String(), "read error") {
		t.Errorf("stderr = %q, want a read error note", stderr.String())
	}
}

func TestRunUnknownNotificationIgnored(t *testing.T) {
	var input bytes.Buffer
	input.Write(frameBytes(t, rpcNotification{JSONRPC: "2.0", Method: "bogus/unknown"}))
	input.Write(shutdownExitFrames(t, 1))

	code, out, _ := runServer(t, input.Bytes())
	if code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
	if strings.Contains(out.String(), "method not found") {
		t.Error("unknown notification (no id) must not get an error response")
	}
}

// TestCompletionBadParamsFallsBackToFullList covers handleCompletion's
// unmarshal-error arm: malformed params do NOT error (as hover/formatting do)
// — completion degrades to the full known-word list (buildCompletionItems).
func TestCompletionBadParamsFallsBackToFullList(t *testing.T) {
	var input bytes.Buffer
	input.Write(frameBytes(t, rpcRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: map[string]any{}}))
	// id=2: params is a bare number, so json.Unmarshal into
	// CompletionParams fails and the handler falls back.
	input.Write(frameBytes(t, rpcRequest{
		JSONRPC: "2.0", ID: 2, Method: "textDocument/completion", Params: 42,
	}))
	input.Write(shutdownExitFrames(t, 9))

	code, out, _ := runServer(t, input.Bytes())
	if code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
	resp := responseFor(2, parseServerOutput(t, out))
	if resp == nil {
		t.Fatal("no response for completion id=2")
	}
	if resp.Error != nil {
		t.Fatalf("completion with bad params = error %+v, want the full word list", resp.Error)
	}
	if items := decodeResult[[]CompletionItem](t, resp.Result); len(items) == 0 {
		t.Error("expected the full known-word completion list on bad params")
	}
}

func TestRunRequestsWithoutIDAreIgnored(t *testing.T) {
	// initialize/hover/completion/formatting all bail out early when
	// the message has no id: nothing may be written for them.
	var input bytes.Buffer
	for _, m := range []string{"initialize", "textDocument/hover", "textDocument/completion", "textDocument/formatting"} {
		input.Write(frameBytes(t, rpcNotification{JSONRPC: "2.0", Method: m}))
	}
	input.Write(frameBytes(t, rpcNotification{JSONRPC: "2.0", Method: "initialized"}))
	input.Write(shutdownExitFrames(t, 1))

	code, out, _ := runServer(t, input.Bytes())
	if code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
	msgs := parseServerOutput(t, out)
	if len(msgs) != 1 { // just the shutdown response
		t.Errorf("expected only the shutdown response, got %d messages", len(msgs))
	}
}

// --- didChange / didClose ---

func TestDidChangeRefreshesDiagnostics(t *testing.T) {
	uri := "file:///tmp/change.aql"
	var input bytes.Buffer
	input.Write(didOpenFrame(t, uri, "upper 42")) // type error → diagnostics
	input.Write(frameBytes(t, rpcNotification{
		JSONRPC: "2.0", Method: "textDocument/didChange",
		Params: DidChangeTextDocumentParams{
			TextDocument:   VersionedTextDocumentIdentifier{URI: uri, Version: 2},
			ContentChanges: []TextDocumentContentChangeEvent{{Text: "1 add 2"}},
		},
	}))
	input.Write(shutdownExitFrames(t, 9))

	code, out, _ := runServer(t, input.Bytes())
	if code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
	pds := diagnosticsNotifications(t, parseServerOutput(t, out))
	if len(pds) != 2 {
		t.Fatalf("expected 2 publishDiagnostics notifications, got %d", len(pds))
	}
	if len(pds[0].Diagnostics) == 0 {
		t.Error("didOpen of a bad buffer should publish diagnostics")
	}
	if len(pds[1].Diagnostics) != 0 {
		t.Errorf("didChange to a clean buffer should clear diagnostics, got %+v", pds[1].Diagnostics)
	}
}

func TestDidChangeLastContentChangeWins(t *testing.T) {
	uri := "file:///tmp/multi.aql"
	last := "1   add    2"
	var input bytes.Buffer
	input.Write(didOpenFrame(t, uri, "1 add 2"))
	input.Write(frameBytes(t, rpcNotification{
		JSONRPC: "2.0", Method: "textDocument/didChange",
		Params: DidChangeTextDocumentParams{
			TextDocument: VersionedTextDocumentIdentifier{URI: uri, Version: 2},
			ContentChanges: []TextDocumentContentChangeEvent{
				{Text: "ignored intermediate"},
				{Text: last},
			},
		},
	}))
	input.Write(frameBytes(t, rpcRequest{
		JSONRPC: "2.0", ID: 5, Method: "textDocument/formatting",
		Params: DocumentFormattingParams{TextDocument: TextDocumentIdentifier{URI: uri}},
	}))
	input.Write(shutdownExitFrames(t, 9))

	code, out, _ := runServer(t, input.Bytes())
	if code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
	resp := responseFor(5, parseServerOutput(t, out))
	if resp == nil {
		t.Fatal("no response for formatting request")
	}
	var edits []TextEdit
	if err := json.Unmarshal(resp.Result, &edits); err != nil {
		t.Fatalf("decode edits: %s", err)
	}
	if len(edits) != 1 {
		t.Fatalf("expected 1 edit, got %d", len(edits))
	}
	if want := formatter.Format(last); edits[0].NewText != want {
		t.Errorf("NewText = %q, want %q (buffer must reflect the LAST change)", edits[0].NewText, want)
	}
}

func TestDidCloseClearsDiagnosticsAndBuffer(t *testing.T) {
	uri := "file:///tmp/close.aql"
	var input bytes.Buffer
	input.Write(didOpenFrame(t, uri, "upper 42"))
	input.Write(frameBytes(t, rpcNotification{
		JSONRPC: "2.0", Method: "textDocument/didClose",
		Params: DidCloseTextDocumentParams{TextDocument: TextDocumentIdentifier{URI: uri}},
	}))
	// Hover on the closed buffer proves the cache entry is gone.
	input.Write(frameBytes(t, rpcRequest{
		JSONRPC: "2.0", ID: 3, Method: "textDocument/hover",
		Params: HoverParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Position:     Position{Line: 0, Character: 1},
		},
	}))
	input.Write(shutdownExitFrames(t, 9))

	code, out, _ := runServer(t, input.Bytes())
	if code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
	msgs := parseServerOutput(t, out)

	pds := diagnosticsNotifications(t, msgs)
	if len(pds) != 2 {
		t.Fatalf("expected 2 publishDiagnostics (open + close), got %d", len(pds))
	}
	if len(pds[0].Diagnostics) == 0 {
		t.Error("didOpen of a bad buffer should publish diagnostics")
	}
	if len(pds[1].Diagnostics) != 0 {
		t.Errorf("didClose must publish an EMPTY diagnostics list, got %+v", pds[1].Diagnostics)
	}
	if pds[1].URI != uri {
		t.Errorf("didClose diagnostics URI = %q, want %q", pds[1].URI, uri)
	}

	resp := responseFor(3, msgs)
	if resp == nil {
		t.Fatal("no response for hover request")
	}
	if string(resp.Result) != "null" {
		t.Errorf("hover on a closed buffer = %s, want null", resp.Result)
	}
}

func TestDidNotificationsWithBadParamsAreLogged(t *testing.T) {
	var input bytes.Buffer
	for _, m := range []string{"textDocument/didOpen", "textDocument/didChange", "textDocument/didClose"} {
		input.Write(frameBytes(t, rpcNotification{JSONRPC: "2.0", Method: m, Params: 42}))
	}
	input.Write(shutdownExitFrames(t, 1))

	code, _, stderr := runServer(t, input.Bytes())
	if code != 0 {
		t.Errorf("run() = %d, want 0 (bad notifications are non-fatal)", code)
	}
	for _, want := range []string{"didOpen decode", "didChange decode", "didClose decode"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr = %q, want to contain %q", stderr.String(), want)
		}
	}
}

// --- hover / formatting request edges ---

func TestHoverRequestEdges(t *testing.T) {
	uri := "file:///tmp/hoveredge.aql"
	var input bytes.Buffer
	input.Write(didOpenFrame(t, uri, "1 add 2"))
	// id=1: unknown buffer → null.
	input.Write(frameBytes(t, rpcRequest{
		JSONRPC: "2.0", ID: 1, Method: "textDocument/hover",
		Params: HoverParams{TextDocument: TextDocumentIdentifier{URI: "file:///nope"}, Position: Position{}},
	}))
	// id=2: cursor on "1" — a word with no help entry → null.
	input.Write(frameBytes(t, rpcRequest{
		JSONRPC: "2.0", ID: 2, Method: "textDocument/hover",
		Params: HoverParams{TextDocument: TextDocumentIdentifier{URI: uri}, Position: Position{Line: 0, Character: 1}},
	}))
	// id=3: malformed params → invalid-request error.
	input.Write(frameBytes(t, rpcRequest{
		JSONRPC: "2.0", ID: 3, Method: "textDocument/hover", Params: 42,
	}))
	// id=4: cursor on "add" → real hover (also exercises the cached
	// registry path, since id=2 built it already).
	input.Write(frameBytes(t, rpcRequest{
		JSONRPC: "2.0", ID: 4, Method: "textDocument/hover",
		Params: HoverParams{TextDocument: TextDocumentIdentifier{URI: uri}, Position: Position{Line: 0, Character: 3}},
	}))
	input.Write(shutdownExitFrames(t, 9))

	code, out, _ := runServer(t, input.Bytes())
	if code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
	msgs := parseServerOutput(t, out)

	for _, id := range []int{1, 2} {
		resp := responseFor(id, msgs)
		if resp == nil {
			t.Fatalf("no response for hover id=%d", id)
		}
		if string(resp.Result) != "null" {
			t.Errorf("hover id=%d = %s, want null", id, resp.Result)
		}
	}

	resp3 := responseFor(3, msgs)
	if resp3 == nil {
		t.Fatal("no response for hover id=3")
	}
	if resp3.Error == nil || resp3.Error.Code != errInvalidRequest {
		t.Errorf("hover with bad params = %+v, want invalid-request error", resp3)
	}

	resp4 := responseFor(4, msgs)
	if resp4 == nil {
		t.Fatal("no response for hover id=4")
	}
	var hover Hover
	if err := json.Unmarshal(resp4.Result, &hover); err != nil {
		t.Fatalf("decode hover: %s (raw %s)", err, resp4.Result)
	}
	if !strings.Contains(hover.Contents.Value, "add") {
		t.Errorf("hover on add = %q, want to mention add", hover.Contents.Value)
	}
}

func TestFormattingRequestEdges(t *testing.T) {
	uri := "file:///tmp/fmtedge.aql"
	clean := formatter.Format("1   add 2") // idempotent input → no edits
	var input bytes.Buffer
	input.Write(didOpenFrame(t, uri, clean))
	// id=1: already-formatted buffer → zero edits.
	input.Write(frameBytes(t, rpcRequest{
		JSONRPC: "2.0", ID: 1, Method: "textDocument/formatting",
		Params: DocumentFormattingParams{TextDocument: TextDocumentIdentifier{URI: uri}},
	}))
	// id=2: unknown buffer → zero edits.
	input.Write(frameBytes(t, rpcRequest{
		JSONRPC: "2.0", ID: 2, Method: "textDocument/formatting",
		Params: DocumentFormattingParams{TextDocument: TextDocumentIdentifier{URI: "file:///nope"}},
	}))
	// id=3: malformed params → invalid-request error.
	input.Write(frameBytes(t, rpcRequest{
		JSONRPC: "2.0", ID: 3, Method: "textDocument/formatting", Params: 42,
	}))
	input.Write(shutdownExitFrames(t, 9))

	code, out, _ := runServer(t, input.Bytes())
	if code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
	msgs := parseServerOutput(t, out)

	for _, id := range []int{1, 2} {
		resp := responseFor(id, msgs)
		if resp == nil {
			t.Fatalf("no response for formatting id=%d", id)
		}
		var edits []TextEdit
		if err := json.Unmarshal(resp.Result, &edits); err != nil {
			t.Fatalf("decode edits id=%d: %s", id, err)
		}
		if len(edits) != 0 {
			t.Errorf("formatting id=%d returned %d edits, want 0", id, len(edits))
		}
	}

	resp3 := responseFor(3, msgs)
	if resp3 == nil {
		t.Fatal("no response for formatting id=3")
	}
	if resp3.Error == nil || resp3.Error.Code != errInvalidRequest {
		t.Errorf("formatting with bad params = %+v, want invalid-request error", resp3)
	}
}
