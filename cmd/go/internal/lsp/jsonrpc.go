// Minimal JSON-RPC 2.0 over the LSP base protocol.
//
// The LSP base protocol frames every payload with
// `Content-Length: N\r\n\r\n<json>`. Headers are CRLF-delimited;
// the only one we care about is Content-Length. We deliberately
// ignore the optional Content-Type header — it defaults to UTF-8
// JSON, which is what we always speak.

package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// rawMessage is the on-the-wire shape: any of request/notification/
// response. We decode the discriminator fields first and then re-
// decode params/result into typed structs as needed.
type rawMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// rpcError matches JSON-RPC 2.0 error semantics.
type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Standard JSON-RPC error codes used by LSP.
const (
	errParseError     = -32700
	errInvalidRequest = -32600
	errMethodNotFound = -32601
	errInternalError  = -32603
)

// hasID reports whether the message carries an id, distinguishing
// requests (which expect a response) from notifications (which do
// not). null id is allowed by JSON-RPC for responses, but for
// requests we only need "id is non-empty".
func (m *rawMessage) hasID() bool {
	if len(m.ID) == 0 {
		return false
	}
	return !bytes.Equal(bytes.TrimSpace(m.ID), []byte("null"))
}

// conn is the JSON-RPC 2.0 transport. It serialises writes through
// mu so notifications from background work cannot interleave with
// responses.
type conn struct {
	br *bufio.Reader
	w  io.Writer
	mu sync.Mutex
}

func newConn(r io.Reader, w io.Writer) *conn {
	return &conn{br: bufio.NewReader(r), w: w}
}

// readMessage blocks until a complete framed message is available
// or the underlying reader returns an error. It returns the raw
// JSON bytes for the caller to decode.
func (c *conn) readMessage() ([]byte, error) {
	contentLength := -1
	for {
		line, err := c.br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break // end of headers
		}
		if i := strings.IndexByte(line, ':'); i >= 0 {
			key := strings.TrimSpace(line[:i])
			val := strings.TrimSpace(line[i+1:])
			if strings.EqualFold(key, "Content-Length") {
				n, perr := strconv.Atoi(val)
				if perr != nil {
					return nil, fmt.Errorf("invalid Content-Length %q: %s", val, perr)
				}
				contentLength = n
			}
		}
	}
	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	buf := make([]byte, contentLength)
	if _, err := io.ReadFull(c.br, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// writeMessage frames payload with the LSP base protocol header and
// writes it. Safe to call from multiple goroutines.
func (c *conn) writeMessage(payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(payload))
	if _, err := io.WriteString(c.w, header); err != nil {
		return err
	}
	_, err := c.w.Write(payload)
	return err
}

// respond writes a successful JSON-RPC response for the given id.
// result may be nil (encoded as "null"), any value that marshals to
// JSON, or a json.RawMessage to splice in pre-encoded bytes.
func (c *conn) respond(id json.RawMessage, result any) error {
	resBytes, err := json.Marshal(result)
	if err != nil {
		return err
	}
	msg := rawMessage{
		JSONRPC: "2.0",
		ID:      id,
		Result:  resBytes,
	}
	out, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return c.writeMessage(out)
}

// respondError writes a JSON-RPC error response for the given id.
func (c *conn) respondError(id json.RawMessage, code int, message string) error {
	msg := rawMessage{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: message},
	}
	out, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return c.writeMessage(out)
}

// notify writes a JSON-RPC notification (no id, no response expected).
func (c *conn) notify(method string, params any) error {
	pBytes, err := json.Marshal(params)
	if err != nil {
		return err
	}
	msg := rawMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  pBytes,
	}
	out, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return c.writeMessage(out)
}
