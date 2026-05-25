// Package ctl implements `aql ctl <op> [name]` — the HTTP client
// for the `api` service. It reads the api's discovery file (or
// --api/--token overrides) and translates ops into REST calls.
//
// Ops match the api's allowed action set:
//
//	aql ctl status              GET  /v1/services
//	aql ctl info                GET  /v1/server
//	aql ctl pause  <svc>        POST /v1/services/<svc>/actions {"action":"pause"}
//	aql ctl resume <svc>        POST /v1/services/<svc>/actions {"action":"resume"}
//	aql ctl stop   <svc>        POST /v1/services/<svc>/actions {"action":"stop"}
package ctl

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aql-lang/aql/cmd/go/internal/api"
	"github.com/aql-lang/aql/cmd/go/internal/command"
)

type cmd struct{}

// New returns the ctl subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "ctl" }
func (*cmd) Synopsis() string { return "control a running `aql serve` process via its api service" }

// Run handles `aql ctl [--api url] [--token tok] <op> [name]`.
func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ctl", flag.ContinueOnError)
	fs.SetOutput(stderr)
	apiURL := fs.String("api", "", "api base URL (default: read from discovery file)")
	token := fs.String("token", "", "bearer token (default: read from discovery file)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	rest := fs.Args()
	if len(rest) == 0 {
		printUsage(stderr)
		return 1
	}

	op := rest[0]
	var name string
	if len(rest) > 1 {
		name = rest[1]
	}

	if *apiURL == "" || *token == "" {
		url, tok, _, err := api.ReadDiscoveryFile()
		if err != nil {
			fmt.Fprintf(stderr, "ctl: %s\n", err)
			return 1
		}
		if *apiURL == "" {
			*apiURL = url
		}
		if *token == "" {
			*token = tok
		}
	}

	switch op {
	case "status":
		return doStatus(*apiURL, *token, stdout, stderr)
	case "info":
		return doInfo(*apiURL, *token, stdout, stderr)
	case "pause", "resume", "stop":
		if name == "" {
			fmt.Fprintf(stderr, "ctl: %s requires a service name\n", op)
			return 1
		}
		return doAction(*apiURL, *token, name, op, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "ctl: unknown op %q\n", op)
		printUsage(stderr)
		return 1
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: aql ctl [--api url] [--token tok] <op> [name]")
	fmt.Fprintln(w, "Ops:")
	fmt.Fprintln(w, "  status              list services and their state")
	fmt.Fprintln(w, "  info                supervisor info (pid, uptime, version)")
	fmt.Fprintln(w, "  pause <svc>         pause a pausable service")
	fmt.Fprintln(w, "  resume <svc>        resume a paused service")
	fmt.Fprintln(w, "  stop <svc>          stop a running service")
}

// doStatus issues GET /v1/services and prints a compact table.
func doStatus(apiURL, token string, stdout, stderr io.Writer) int {
	var svcs []serviceEntity
	if err := getJSON(apiURL+"/v1/services", token, &svcs); err != nil {
		fmt.Fprintf(stderr, "ctl: %s\n", err)
		return 1
	}
	for _, s := range svcs {
		extra := []string{}
		for k, v := range s.Metadata {
			extra = append(extra, k+"="+v)
		}
		meta := ""
		if len(extra) > 0 {
			meta = "  (" + strings.Join(extra, " ") + ")"
		}
		fmt.Fprintf(stdout, "%-12s %s%s\n", s.Name, s.State, meta)
	}
	return 0
}

// doInfo issues GET /v1/server and prints the supervisor info.
func doInfo(apiURL, token string, stdout, stderr io.Writer) int {
	var info struct {
		PID           int     `json:"pid"`
		Version       string  `json:"version"`
		StartTime     string  `json:"startTime"`
		UptimeSeconds float64 `json:"uptimeSeconds"`
		ServiceCount  int     `json:"serviceCount"`
	}
	if err := getJSON(apiURL+"/v1/server", token, &info); err != nil {
		fmt.Fprintf(stderr, "ctl: %s\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "pid:      %d\n", info.PID)
	fmt.Fprintf(stdout, "version:  %s\n", info.Version)
	fmt.Fprintf(stdout, "started:  %s\n", info.StartTime)
	fmt.Fprintf(stdout, "uptime:   %.0fs\n", info.UptimeSeconds)
	fmt.Fprintf(stdout, "services: %d\n", info.ServiceCount)
	return 0
}

// doAction issues POST /v1/services/<name>/actions.
//
// Stopping the api service itself is a corner case: the server tears
// down before flushing the response, so the client observes a
// connection-level EOF. We treat that specific case (action=stop and
// transport-level EOF) as success, since the request reached the
// handler and the supervisor will exit cleanly.
func doAction(apiURL, token, name, action string, stdout, stderr io.Writer) int {
	body, _ := json.Marshal(map[string]string{"action": action})
	req, err := http.NewRequest(http.MethodPost, apiURL+"/v1/services/"+name+"/actions", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(stderr, "ctl: %s\n", err)
		return 1
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient().Do(req)
	if err != nil {
		if action == "stop" && isConnectionTorn(err) {
			fmt.Fprintln(stdout, "ok")
			return 0
		}
		fmt.Fprintf(stderr, "ctl: %s\n", err)
		return 1
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
		fmt.Fprintf(stderr, "ctl: %s\n", e.Error)
		return 1
	}
	fmt.Fprintln(stdout, "ok")
	return 0
}

// isConnectionTorn reports whether err looks like the server cut the
// connection mid-response (EOF, connection reset). Used to handle
// the "stop the api itself" case gracefully.
func isConnectionTorn(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "EOF") || strings.Contains(s, "connection reset")
}

// serviceEntity matches the Service schema from openapi.yaml.
type serviceEntity struct {
	Name      string            `json:"name"`
	State     string            `json:"state"`
	Pausable  bool              `json:"pausable"`
	UsesStdio bool              `json:"usesStdio"`
	Metadata  map[string]string `json:"metadata"`
}

func getJSON(url, token string, out any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("api %s: %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func httpClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Second}
}
