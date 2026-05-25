// client.go is the HTTP client used by the bubbletea model to talk
// to the api service. Kept separate so the model is purely UI logic.

package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// apiClient is a tiny wrapper around http.Client that adds the
// bearer token and JSON decoding.
type apiClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func newAPIClient(baseURL, token string) *apiClient {
	return &apiClient{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: 3 * time.Second},
	}
}

// serviceEntity matches the Service schema in openapi.yaml.
type serviceEntity struct {
	Name      string            `json:"name"`
	State     string            `json:"state"`
	Pausable  bool              `json:"pausable"`
	UsesStdio bool              `json:"usesStdio"`
	Metadata  map[string]string `json:"metadata"`
}

// serverInfo matches the Server schema in openapi.yaml.
type serverInfo struct {
	PID           int     `json:"pid"`
	Version       string  `json:"version"`
	StartTime     string  `json:"startTime"`
	UptimeSeconds float64 `json:"uptimeSeconds"`
	ServiceCount  int     `json:"serviceCount"`
}

// listServices issues GET /v1/services.
func (c *apiClient) listServices() ([]serviceEntity, error) {
	var out []serviceEntity
	if err := c.getJSON("/v1/services", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// getServer issues GET /v1/server.
func (c *apiClient) getServer() (*serverInfo, error) {
	var out serverInfo
	if err := c.getJSON("/v1/server", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// applyAction issues POST /v1/services/<name>/actions.
func (c *apiClient) applyAction(name, action string) error {
	body, _ := json.Marshal(map[string]string{"action": action})
	req, _ := http.NewRequest(http.MethodPost, c.baseURL+"/v1/services/"+name+"/actions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.addAuth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return fmt.Errorf("%s", e.Error)
	}
	return nil
}

func (c *apiClient) getJSON(path string, out any) error {
	req, _ := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	c.addAuth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s %s: %s", req.Method, path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *apiClient) addAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}
