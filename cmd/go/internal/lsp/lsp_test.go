package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aql-lang/aql/lang/go/formatter"
)

// client drives the LSP server in tests via in-memory pipes. A
// background goroutine continuously reads framed messages from the
// server so that the server's writeMessage calls never block on a
// missing reader (which is what caused the io.Pipe deadlock the
// naive test harness hit).
type client struct {
	t        *testing.T
	writer   *io.PipeWriter
	srvOut   *io.PipeWriter // we hold this so we can close it after exit
	stderr   bytes.Buffer
	msgs     chan rawMessage
	readDone chan struct{}
	srvDone  chan struct{}
}

func newClient(t *testing.T) *client {
	t.Helper()
	serverIn, clientWriter := io.Pipe()
	clientReader, serverOut := io.Pipe()

	c := &client{
		t:        t,
		writer:   clientWriter,
		srvOut:   serverOut,
		msgs:     make(chan rawMessage, 64),
		readDone: make(chan struct{}),
		srvDone:  make(chan struct{}),
	}

	// Server goroutine: read from clientWriter (via serverIn),
	// write to serverOut (read by clientReader).
	go func() {
		defer close(c.srvDone)
		s := newServer(serverIn, serverOut, &c.stderr)
		s.run()
		// run() returned (likely on "exit" notification). Close
		// serverOut so the reader goroutine sees EOF.
		serverOut.Close()
	}()

	// Reader goroutine: pull framed messages out of the server and
	// push them into c.msgs. Exits on EOF.
	go func() {
		defer close(c.readDone)
		br := bufio.NewReader(clientReader)
		for {
			data, err := readFramedMessage(br)
			if err != nil {
				return
			}
			var m rawMessage
			if jerr := json.Unmarshal(data, &m); jerr != nil {
				continue
			}
			c.msgs <- m
		}
	}()

	return c
}

// readFramedMessage reads one LSP base-protocol framed message.
// Returns io.EOF when the underlying pipe closes.
func readFramedMessage(br *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if i := strings.IndexByte(line, ':'); i >= 0 {
			key := strings.TrimSpace(line[:i])
			val := strings.TrimSpace(line[i+1:])
			if strings.EqualFold(key, "Content-Length") {
				n, perr := strconv.Atoi(val)
				if perr != nil {
					return nil, fmt.Errorf("invalid Content-Length: %s", perr)
				}
				contentLength = n
			}
		}
	}
	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length")
	}
	buf := make([]byte, contentLength)
	if _, err := io.ReadFull(br, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// send writes one JSON-RPC message (request or notification) to the
// server.
func (c *client) send(payload any) {
	c.t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		c.t.Fatal(err)
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(c.writer, header); err != nil {
		c.t.Fatalf("write header: %s", err)
	}
	if _, err := c.writer.Write(body); err != nil {
		c.t.Fatalf("write body: %s", err)
	}
}

// recv pulls the next message from the reader queue, with a timeout
// long enough for the slowest sync operation (lang.Check on first
// invocation builds the registry).
func (c *client) recv() rawMessage {
	c.t.Helper()
	select {
	case m := <-c.msgs:
		return m
	case <-time.After(5 * time.Second):
		c.t.Fatal("timeout waiting for server message")
	}
	return rawMessage{}
}

// recvResponse pulls messages until one with the given id arrives.
// Notifications received before the response are buffered into the
// returned slice in receipt order.
func (c *client) recvResponse(id int) (rawMessage, []rawMessage) {
	c.t.Helper()
	var before []rawMessage
	for {
		m := c.recv()
		if m.hasID() {
			var got int
			if json.Unmarshal(m.ID, &got) == nil && got == id {
				return m, before
			}
		}
		before = append(before, m)
	}
}

// shutdown sends shutdown + exit and waits for the server to stop.
// Safe to call once per client; double-close panics on the pipe.
func (c *client) shutdown() {
	c.t.Helper()
	c.send(rpcRequest{JSONRPC: "2.0", ID: 9999, Method: "shutdown"})
	c.recvResponse(9999)
	c.send(rpcNotification{JSONRPC: "2.0", Method: "exit"})
	c.writer.Close()
	select {
	case <-c.srvDone:
	case <-time.After(3 * time.Second):
		c.t.Fatal("server did not exit")
	}
	<-c.readDone
	if c.t.Failed() && c.stderr.Len() > 0 {
		c.t.Logf("server stderr:\n%s", c.stderr.String())
	}
}

// rpcRequest is a JSON-RPC request payload.
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// rpcNotification is a JSON-RPC notification payload.
type rpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// --- helpers to decode typed payloads ---

func decodeResult[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode: %s", err)
	}
	return v
}

func notificationFor(method string, msgs []rawMessage) *rawMessage {
	for i := range msgs {
		if !msgs[i].hasID() && msgs[i].Method == method {
			return &msgs[i]
		}
	}
	return nil
}

// --- tests ---

func TestInitializeReportsCapabilities(t *testing.T) {
	c := newClient(t)
	defer c.shutdown()

	c.send(rpcRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: map[string]any{}})
	resp, _ := c.recvResponse(1)

	res := decodeResult[InitializeResult](t, resp.Result)
	if res.Capabilities.TextDocumentSync != syncFull {
		t.Errorf("TextDocumentSync = %d, want %d", res.Capabilities.TextDocumentSync, syncFull)
	}
	if !res.Capabilities.HoverProvider {
		t.Error("expected HoverProvider=true")
	}
	if res.Capabilities.CompletionProvider == nil {
		t.Error("expected CompletionProvider != nil")
	}
	if !res.Capabilities.DocumentFormattingProvider {
		t.Error("expected DocumentFormattingProvider=true")
	}
}

func TestDidOpenPublishesDiagnostics(t *testing.T) {
	c := newClient(t)
	defer c.shutdown()

	c.send(rpcRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: map[string]any{}})
	c.recvResponse(1)

	uri := "file:///tmp/bad.aql"
	c.send(rpcNotification{
		JSONRPC: "2.0",
		Method:  "textDocument/didOpen",
		Params: DidOpenTextDocumentParams{
			TextDocument: TextDocumentItem{
				URI: uri, LanguageID: "aql", Version: 1, Text: "upper 42",
			},
		},
	})

	// Drive a no-op request to give us a synchronisation point —
	// the publishDiagnostics notification fires from inside
	// handleDidOpen, so by the time the response to id=2 arrives,
	// the notification has already been sent.
	c.send(rpcRequest{
		JSONRPC: "2.0", ID: 2, Method: "textDocument/formatting",
		Params: DocumentFormattingParams{TextDocument: TextDocumentIdentifier{URI: uri}},
	})
	_, before := c.recvResponse(2)

	note := notificationFor("textDocument/publishDiagnostics", before)
	if note == nil {
		t.Fatalf("no publishDiagnostics notification (got %d messages)", len(before))
	}
	pd := decodeResult[PublishDiagnosticsParams](t, note.Params)
	if pd.URI != uri {
		t.Errorf("URI = %q, want %q", pd.URI, uri)
	}
	if len(pd.Diagnostics) == 0 {
		t.Fatal("expected at least one diagnostic for 'upper 42'")
	}
	hasError := false
	for _, d := range pd.Diagnostics {
		if d.Severity == severityError {
			hasError = true
			if d.Code == "" {
				t.Errorf("diagnostic has empty code: %+v", d)
			}
		}
	}
	if !hasError {
		t.Errorf("no error-severity diagnostic in %+v", pd.Diagnostics)
	}
}

func TestHoverReturnsHelp(t *testing.T) {
	c := newClient(t)
	defer c.shutdown()

	c.send(rpcRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: map[string]any{}})
	c.recvResponse(1)

	uri := "file:///tmp/hover.aql"
	c.send(rpcNotification{
		JSONRPC: "2.0",
		Method:  "textDocument/didOpen",
		Params: DidOpenTextDocumentParams{
			TextDocument: TextDocumentItem{URI: uri, LanguageID: "aql", Version: 1, Text: "1 add 2"},
		},
	})

	c.send(rpcRequest{
		JSONRPC: "2.0", ID: 2, Method: "textDocument/hover",
		Params: HoverParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Position:     Position{Line: 0, Character: 3},
		},
	})
	resp, _ := c.recvResponse(2)
	if len(resp.Result) == 0 || string(resp.Result) == "null" {
		t.Fatalf("hover result is null/empty: %s", string(resp.Result))
	}
	hover := decodeResult[Hover](t, resp.Result)
	if !strings.Contains(hover.Contents.Value, "Signatures:") {
		t.Errorf("expected 'Signatures:' in hover, got:\n%s", hover.Contents.Value)
	}
}

func TestCompletionLists(t *testing.T) {
	c := newClient(t)
	defer c.shutdown()

	c.send(rpcRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: map[string]any{}})
	c.recvResponse(1)

	uri := "file:///tmp/c.aql"
	c.send(rpcNotification{
		JSONRPC: "2.0",
		Method:  "textDocument/didOpen",
		Params: DidOpenTextDocumentParams{
			TextDocument: TextDocumentItem{URI: uri, LanguageID: "aql", Version: 1, Text: ""},
		},
	})

	c.send(rpcRequest{
		JSONRPC: "2.0", ID: 2, Method: "textDocument/completion",
		Params: CompletionParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Position:     Position{Line: 0, Character: 0},
		},
	})
	resp, _ := c.recvResponse(2)
	items := decodeResult[[]CompletionItem](t, resp.Result)
	if len(items) == 0 {
		t.Fatal("expected non-empty completion list")
	}

	seen := map[string]bool{}
	for _, it := range items {
		seen[it.Label] = true
	}
	for _, want := range []string{"add", "upper", "concat"} {
		if !seen[want] {
			t.Errorf("completion missing %q (have %d items)", want, len(items))
		}
	}
}

func TestFormattingProducesTextEdit(t *testing.T) {
	c := newClient(t)
	defer c.shutdown()

	c.send(rpcRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: map[string]any{}})
	c.recvResponse(1)

	uri := "file:///tmp/fmt.aql"
	src := "1   add    2"
	want := formatter.Format(src)

	c.send(rpcNotification{
		JSONRPC: "2.0",
		Method:  "textDocument/didOpen",
		Params: DidOpenTextDocumentParams{
			TextDocument: TextDocumentItem{URI: uri, LanguageID: "aql", Version: 1, Text: src},
		},
	})

	c.send(rpcRequest{
		JSONRPC: "2.0", ID: 2, Method: "textDocument/formatting",
		Params: DocumentFormattingParams{TextDocument: TextDocumentIdentifier{URI: uri}},
	})
	resp, _ := c.recvResponse(2)
	edits := decodeResult[[]TextEdit](t, resp.Result)
	if want != src {
		if len(edits) != 1 {
			t.Fatalf("expected 1 edit, got %d", len(edits))
		}
		if edits[0].NewText != want {
			t.Errorf("NewText = %q, want %q", edits[0].NewText, want)
		}
	} else {
		if len(edits) != 0 {
			t.Errorf("expected 0 edits (already formatted), got %d", len(edits))
		}
	}
}

func TestUnknownMethodReturnsError(t *testing.T) {
	c := newClient(t)
	defer c.shutdown()

	c.send(rpcRequest{JSONRPC: "2.0", ID: 1, Method: "totally/unknown"})
	resp, _ := c.recvResponse(1)
	if resp.Error == nil {
		t.Fatalf("expected error response, got result: %s", string(resp.Result))
	}
	if resp.Error.Code != errMethodNotFound {
		t.Errorf("error code = %d, want %d", resp.Error.Code, errMethodNotFound)
	}
}

// TestParseErrorReportedAsDiagnostic verifies that a buffer the
// parser rejects (e.g. an unterminated string) still triggers a
// publishDiagnostics notification with at least one error-severity
// entry — i.e. the lang.Check error path is surfaced, not silently
// swallowed.
func TestParseErrorReportedAsDiagnostic(t *testing.T) {
	c := newClient(t)
	defer c.shutdown()

	c.send(rpcRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: map[string]any{}})
	c.recvResponse(1)

	uri := "file:///tmp/parse.aql"
	c.send(rpcNotification{
		JSONRPC: "2.0",
		Method:  "textDocument/didOpen",
		Params: DidOpenTextDocumentParams{
			TextDocument: TextDocumentItem{
				URI: uri, LanguageID: "aql", Version: 1, Text: `"unterminated`,
			},
		},
	})

	c.send(rpcRequest{
		JSONRPC: "2.0", ID: 2, Method: "textDocument/formatting",
		Params: DocumentFormattingParams{TextDocument: TextDocumentIdentifier{URI: uri}},
	})
	_, before := c.recvResponse(2)

	note := notificationFor("textDocument/publishDiagnostics", before)
	if note == nil {
		t.Fatal("no publishDiagnostics notification for parse error")
	}
	pd := decodeResult[PublishDiagnosticsParams](t, note.Params)
	if len(pd.Diagnostics) == 0 {
		t.Fatal("expected at least one diagnostic for unterminated string")
	}
	hasError := false
	for _, d := range pd.Diagnostics {
		if d.Severity == severityError {
			hasError = true
		}
	}
	if !hasError {
		t.Errorf("no error-severity diagnostic for parse failure: %+v", pd.Diagnostics)
	}
}

// TestWholeDocumentRangeCountsUTF16Units exercises wholeDocumentRange
// directly so the UTF-16-vs-bytes regression has a focused unit test
// independent of the JSON-RPC plumbing.
func TestWholeDocumentRangeCountsUTF16Units(t *testing.T) {
	// 🌍 (U+1F30D) is outside the BMP → 2 UTF-16 code units.
	// "hello " is 6 BMP characters → 6 UTF-16 code units.
	// Total expected end character: 8.
	r := wholeDocumentRange("hello \U0001F30D")
	if r.End.Line != 0 {
		t.Errorf("End.Line = %d, want 0", r.End.Line)
	}
	if r.End.Character != 8 {
		t.Errorf("End.Character = %d, want 8 (6 BMP + 1 surrogate pair)", r.End.Character)
	}

	// Multi-line ASCII keeps the same shape — last-line length, in code units.
	r2 := wholeDocumentRange("abc\ndefg")
	if r2.End.Line != 1 {
		t.Errorf("End.Line = %d, want 1", r2.End.Line)
	}
	if r2.End.Character != 4 {
		t.Errorf("End.Character = %d, want 4", r2.End.Character)
	}
}
