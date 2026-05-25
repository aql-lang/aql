// control.go implements the supervisor-side of the ctl protocol: a
// tiny line-delimited JSON server on a Unix domain socket. Each line
// the client sends is one request; the server replies with one line
// per request and closes when the client closes.
//
// Wire format (one JSON object per line, both directions):
//
//	→ {"op":"status"}
//	← {"ok":true,"services":[{"name":"repl","state":"running"}]}
//	→ {"op":"pause","name":"repl"}
//	← {"ok":true}
//	→ {"op":"stop","name":"lsp"}
//	← {"ok":true}
//
// ops: status (no name), start, stop, pause, resume (all take name).

package serve

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/aql-lang/aql/cmd/go/internal/service"
)

// controlServer hosts the Unix socket. Constructed and owned by the
// supervisor when --ctl is given.
type controlServer struct {
	path string
	ln   net.Listener
	sup  *supervisor
	done chan struct{}
}

// ctlRequest is the wire shape of one control message.
type ctlRequest struct {
	Op   string `json:"op"`
	Name string `json:"name,omitempty"`
}

// ctlStatusEntry is one service entry in a status response.
type ctlStatusEntry struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// ctlResponse is the wire shape of one reply.
type ctlResponse struct {
	OK       bool             `json:"ok"`
	Error    string           `json:"error,omitempty"`
	Services []ctlStatusEntry `json:"services,omitempty"`
}

// defaultCtlPath returns the well-known socket path when --ctl is
// passed with no value. Single fixed path under $TMPDIR so the
// matching `aql ctl` default lines up without arguments; users
// running multiple supervisors must pass --ctl=<path> explicitly.
func defaultCtlPath() string {
	return filepath.Join(os.TempDir(), "aql-serve.sock")
}

// startControlSocket binds the Unix socket and starts the accept
// loop. Returns an error if the socket cannot be created.
func (sup *supervisor) startControlSocket(path string) error {
	// If the path exists from a stale prior run, try to remove it
	// only if it is a socket — never blindly unlink a regular file.
	if info, err := os.Stat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("ctl path %q exists and is not a socket", path)
		}
		_ = os.Remove(path)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	// Restrict to the owner: ctl actions can stop services.
	_ = os.Chmod(path, 0600)

	cs := &controlServer{
		path: path,
		ln:   ln,
		sup:  sup,
		done: make(chan struct{}),
	}
	sup.ctlServer = cs

	go cs.acceptLoop()
	return nil
}

func (cs *controlServer) close() error {
	close(cs.done)
	err := cs.ln.Close()
	_ = os.Remove(cs.path)
	return err
}

// acceptLoop runs until the listener is closed.
func (cs *controlServer) acceptLoop() {
	for {
		conn, err := cs.ln.Accept()
		if err != nil {
			select {
			case <-cs.done:
				return
			default:
			}
			fmt.Fprintf(cs.sup.stderr, "ctl: accept: %s\n", err)
			return
		}
		go cs.handleConn(conn)
	}
}

// handleConn services one client until they close the connection.
// Each line is one request/response pair.
func (cs *controlServer) handleConn(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 4096), 64*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var req ctlRequest
		if err := json.Unmarshal(line, &req); err != nil {
			writeReply(conn, ctlResponse{Error: "invalid request: " + err.Error()})
			continue
		}
		writeReply(conn, cs.dispatch(req))
	}
}

func writeReply(conn net.Conn, resp ctlResponse) {
	if resp.Error == "" {
		resp.OK = true
	}
	b, _ := json.Marshal(resp)
	b = append(b, '\n')
	_, _ = conn.Write(b)
}

// dispatch executes one parsed request against the supervisor.
func (cs *controlServer) dispatch(req ctlRequest) ctlResponse {
	switch req.Op {
	case "status":
		entries := make([]ctlStatusEntry, 0, len(cs.sup.services))
		for _, s := range cs.sup.services {
			entries = append(entries, ctlStatusEntry{Name: s.Name(), State: s.Status().String()})
		}
		return ctlResponse{Services: entries}
	case "stop":
		svc, ok := cs.sup.byName[req.Name]
		if !ok {
			return ctlResponse{Error: "unknown service: " + req.Name}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := svc.Stop(ctx); err != nil {
			return ctlResponse{Error: err.Error()}
		}
		// Also cancel its context so Start unwinds.
		cs.sup.mu.Lock()
		if c, ok := cs.sup.cancels[req.Name]; ok {
			c()
		}
		cs.sup.mu.Unlock()
		return ctlResponse{}
	case "pause":
		return cs.applyPausable(req.Name, true)
	case "resume":
		return cs.applyPausable(req.Name, false)
	case "start":
		// Restart a previously-stopped service. Out of scope for v1:
		// it would require holding a factory + args alongside each
		// Service. Document the limitation rather than half-implement.
		return ctlResponse{Error: "start: restarting stopped services is not supported in v1"}
	default:
		return ctlResponse{Error: "unknown op: " + req.Op}
	}
}

func (cs *controlServer) applyPausable(name string, pause bool) ctlResponse {
	svc, ok := cs.sup.byName[name]
	if !ok {
		return ctlResponse{Error: "unknown service: " + name}
	}
	p, ok := svc.(service.Pausable)
	if !ok {
		return ctlResponse{Error: name + " does not support pause/resume"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var err error
	if pause {
		err = p.Pause(ctx)
	} else {
		err = p.Resume(ctx)
	}
	if err != nil {
		return ctlResponse{Error: err.Error()}
	}
	return ctlResponse{}
}
