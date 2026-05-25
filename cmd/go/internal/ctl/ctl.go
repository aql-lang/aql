// Package ctl implements `aql ctl <op> [name]` — the client side of
// the supervisor's control socket. It connects to a Unix socket
// opened by `aql serve --ctl`, sends one JSON line, prints the JSON
// reply, and exits.
//
// Default socket path matches serve's default: $TMPDIR/aql-serve-<pid>.sock.
// Since the PID is per-supervisor, callers will usually pass --socket
// explicitly; the default is convenient for the common single-process
// case (you can copy the path serve prints at startup).
//
// Ops: status, pause <svc>, resume <svc>, stop <svc>.
package ctl

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/aql-lang/aql/cmd/go/internal/command"
)

type cmd struct{}

// New returns the ctl subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "ctl" }
func (*cmd) Synopsis() string { return "control a running `aql serve` process" }

// Run handles `aql ctl [--socket path] <op> [name]`.
func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ctl", flag.ContinueOnError)
	fs.SetOutput(stderr)
	socket := fs.String("socket", "", "path to the supervisor control socket")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(stderr, "usage: aql ctl [--socket path] <op> [name]")
		fmt.Fprintln(stderr, "ops: status, pause <svc>, resume <svc>, stop <svc>")
		return 1
	}

	op := rest[0]
	var name string
	if len(rest) > 1 {
		name = rest[1]
	}

	switch op {
	case "status":
		if name != "" {
			fmt.Fprintln(stderr, "ctl: status takes no argument")
			return 1
		}
	case "pause", "resume", "stop":
		if name == "" {
			fmt.Fprintf(stderr, "ctl: %s requires a service name\n", op)
			return 1
		}
	default:
		fmt.Fprintf(stderr, "ctl: unknown op %q\n", op)
		return 1
	}

	path := *socket
	if path == "" {
		path = defaultSocket()
	}

	conn, err := net.DialTimeout("unix", path, 3*time.Second)
	if err != nil {
		fmt.Fprintf(stderr, "ctl: dial %s: %s\n", path, err)
		return 1
	}
	defer conn.Close()

	req := map[string]any{"op": op}
	if name != "" {
		req["name"] = name
	}
	reqBytes, _ := json.Marshal(req)
	reqBytes = append(reqBytes, '\n')
	if _, err := conn.Write(reqBytes); err != nil {
		fmt.Fprintf(stderr, "ctl: write: %s\n", err)
		return 1
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil && err != io.EOF {
		fmt.Fprintf(stderr, "ctl: read: %s\n", err)
		return 1
	}

	var resp struct {
		OK       bool             `json:"ok"`
		Error    string           `json:"error,omitempty"`
		Services []map[string]any `json:"services,omitempty"`
	}
	if err := json.Unmarshal(line, &resp); err != nil {
		fmt.Fprintf(stderr, "ctl: invalid reply: %s\n", err)
		return 1
	}
	if !resp.OK {
		fmt.Fprintf(stderr, "ctl: %s\n", resp.Error)
		return 1
	}

	if op == "status" {
		for _, s := range resp.Services {
			fmt.Fprintf(stdout, "%-12s %s\n", s["name"], s["state"])
		}
		return 0
	}
	fmt.Fprintf(stdout, "ok\n")
	return 0
}

// defaultSocket matches serve's default ctl path so the no-arg case
// (one supervisor, default --ctl) works without explicit --socket.
func defaultSocket() string {
	return filepath.Join(os.TempDir(), "aql-serve.sock")
}
