package debugserve

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client attaches to a debugserve.Server over HTTP — the §7.2 "attach to a
// running runtime" leg. It carries the bearer token (a static token or a
// vault capability id) on every request.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient builds a client for baseURL (e.g. "http://127.0.0.1:8799").
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

// Words returns the remote runtime's built-in word names.
func (c *Client) Words() ([]string, error) {
	var out struct {
		Words []string `json:"words"`
	}
	if err := c.getJSON("/debug/words", &out); err != nil {
		return nil, err
	}
	return out.Words, nil
}

// Defs returns the remote runtime's current def bindings (name -> rendered).
func (c *Client) Defs() (map[string]string, error) {
	var out struct {
		Defs map[string]string `json:"defs"`
	}
	if err := c.getJSON("/debug/defs", &out); err != nil {
		return nil, err
	}
	return out.Defs, nil
}

// Heap returns the remote runtime's heap stats.
func (c *Client) Heap() (HeapStats, error) {
	var out HeapStats
	err := c.getJSON("/debug/heap", &out)
	return out, err
}

// Eval runs src on the remote runtime and returns the rendered result. A
// runtime/parse error in the evaluated source is returned as the second
// value (distinct from a transport error, which is returned as err).
func (c *Client) Eval(src string) (result string, evalErr string, err error) {
	var out struct {
		Result string `json:"result"`
		Error  string `json:"error"`
	}
	if err = c.postJSON("/debug/eval", []byte(src), &out); err != nil {
		return "", "", err
	}
	return out.Result, out.Error, nil
}

// Emit appends a serverless-channel event for invocation id (the function
// side of §7.3).
func (c *Client) Emit(id string, ev Event) error {
	body, _ := json.Marshal(ev)
	var out map[string]any
	return c.postJSON("/debug/emit?id="+url.QueryEscape(id), body, &out)
}

// Events reads the events buffered for invocation id (the interrogator side
// of §7.3).
func (c *Client) Events(id string) ([]Event, error) {
	var out struct {
		Events []Event `json:"events"`
	}
	if err := c.getJSON("/debug/events?id="+url.QueryEscape(id), &out); err != nil {
		return nil, err
	}
	return out.Events, nil
}

func (c *Client) getJSON(path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) postJSON(path string, body []byte, out any) error {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("debug attach: %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	return json.Unmarshal(data, out)
}
