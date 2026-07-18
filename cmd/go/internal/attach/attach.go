// Package attach implements `aql attach <host:port> [--token T]` — the
// thin viewer for a served aql:tui app (design/TUI.0.md §6.4, plan P4).
// It is deliberately dumb: dial, handshake, then loop — widget trees
// come down as json-lines and lay out LOCALLY through the shared tuikit
// renderer onto the real terminal; decoded input events go back up. No
// AQL engine runs client-side.
package attach

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"

	"github.com/aql-lang/aql/cmd/go/internal/command"
	"github.com/aql-lang/aql/cmd/go/internal/termback"
	"github.com/aql-lang/aql/lang/go/tuikit"
)

// Test seams (design/TEST-SEAMS.10.md).
var (
	dialFn     = func(addr string) (net.Conn, error) { return net.Dial("tcp", addr) }
	newBackend = func() tuikit.Backend { return termback.New() }
)

const wireLimit = 4 << 20

type cmd struct{}

// New returns the attach subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string { return "attach" }
func (*cmd) Synopsis() string {
	return "attach to a served aql:tui app and render it on this terminal"
}

// Run handles `aql attach [--token T] <host:port>`.
func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("attach", flag.ContinueOnError)
	fs.SetOutput(stderr)
	token := fs.String("token", "", "the serving app's token (or its token: \"none\" opt-out)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: aql attach [--token T] <host:port>")
		return 1
	}
	if err := attach(fs.Arg(0), *token); err != nil {
		fmt.Fprintf(stderr, "attach: %s\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "session ended")
	return 0
}

func attach(addr, token string) error {
	backend := newBackend()
	info, err := backend.Open(tuikit.OpenOpts{})
	if err != nil {
		return err
	}
	defer func() { _ = backend.Close() }()

	conn, err := dialFn(addr)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	writeLine := func(payload any) error {
		data, mErr := json.Marshal(payload)
		if mErr != nil { //covergate:allow json.Marshal cannot fail on the map[string]any shapes this client builds (strings, ints, bools); kept as the io seam's defensive twin (§misc)
			return mErr
		}
		_, wErr := conn.Write(append(data, '\n'))
		return wErr
	}
	if err := writeLine(map[string]any{
		"tag": "attach", "token": token,
		"cols": info.Cols, "rows": info.Rows, "proto": 1,
	}); err != nil {
		return err
	}

	lines := make(chan map[string]any, 8)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(conn)
		scanner.Buffer(make([]byte, 0, 4096), wireLimit)
		for scanner.Scan() {
			var msg map[string]any
			if json.Unmarshal(scanner.Bytes(), &msg) == nil {
				lines <- msg
			}
		}
	}()

	first, ok := <-lines
	if !ok {
		return fmt.Errorf("connection closed during handshake")
	}
	switch first["tag"] {
	case "accept":
	case "deny":
		return fmt.Errorf("denied: %v", first["why"])
	default:
		return fmt.Errorf("unexpected handshake reply %v", first["tag"])
	}

	for {
		select {
		case ev, evOK := <-backend.Events():
			if !evOK {
				return nil // local terminal gone
			}
			if ev.Tag == "resize" && ev.Cols > 0 && ev.Rows > 0 {
				// later frames lay out at the NEW local geometry; the
				// event still goes upstream so the app can react too
				info.Cols, info.Rows = ev.Cols, ev.Rows
			}
			if err := writeLine(eventWire(ev)); err != nil {
				return err
			}
		case msg, msgOK := <-lines:
			if !msgOK {
				return fmt.Errorf("server closed the connection")
			}
			switch msg["tag"] {
			case "frame":
				res, rErr := tuikit.Render(msg["tree"], info.Cols, info.Rows)
				if rErr != nil {
					return fmt.Errorf("bad frame: %w", rErr)
				}
				if pErr := backend.Present(res.Frame); pErr != nil {
					return pErr
				}
				if res.Cursor != nil {
					if cErr := backend.SetCursor(res.Cursor.X, res.Cursor.Y, true); cErr != nil {
						return cErr
					}
				} else if cErr := backend.SetCursor(0, 0, false); cErr != nil {
					return cErr
				}
			case "title":
				if s, isStr := msg["text"].(string); isStr {
					if tErr := backend.SetTitle(s); tErr != nil {
						return tErr
					}
				}
			case "bell":
				if bErr := backend.Bell(); bErr != nil {
					return bErr
				}
			case "quit":
				return nil
			}
		}
	}
}

// eventWire projects a decoded local event onto the §6.2 wire shape.
func eventWire(ev tuikit.Event) map[string]any {
	m := map[string]any{"tag": ev.Tag}
	switch ev.Tag {
	case "key":
		m["key"], m["char"] = ev.Key, ev.Char
		mods := make([]string, len(ev.Mods))
		copy(mods, ev.Mods)
		m["mods"] = mods
	case "resize":
		m["cols"], m["rows"] = ev.Cols, ev.Rows
	case "mouse":
		m["kind"], m["x"], m["y"] = ev.Kind, ev.X, ev.Y
	case "paste":
		m["text"] = ev.Text
	case "focus":
		m["gained"] = ev.Gained
	}
	return m
}
