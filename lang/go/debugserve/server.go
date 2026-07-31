// Package debugserve is the host-level realization of the cross-process
// debug surfaces of boru:debug (design/DEBUG-MODULE.0.md §7.2 attach and
// §7.3 the serverless debug channel).
//
// The design's eventual unification routes these through the boru `Service`
// model, which does not yet exist (SERVICES.0.md / PROCESSES.0.md are
// RFC-only, and boru has no language-level socket primitive). This package
// is the pragmatic implementation on the substrate that DOES exist: Go's
// net/http, the same bearer-token + discovery-file pattern the api service
// already uses (cmd/go/internal/api), and vault capability tokens for auth.
// It wraps a *native.Registry behind authenticated HTTP introspection so a
// separate process can attach and interrogate a running boru runtime:
//
//	GET  /debug/healthz          liveness
//	GET  /debug/words            the registry's built-in word names
//	GET  /debug/defs             current def bindings (name -> rendered value)
//	GET  /debug/heap             Go-runtime heap stats
//	POST /debug/eval             run boru source on the registry, return the result
//	POST /debug/emit?id=<inv>    (serverless channel) append a debug event for an invocation
//	GET  /debug/events?id=<inv>  (serverless channel) read an invocation's events
//
// The remote STEPPING surface (`boru debug serve --step`, BORU-DEBUGGER.0.md
// §8.3 Phase 4) mounts three more routes over this same transport — the
// handlers live with the debugger session (cmd/go/internal/debugger); this
// package carries the wire shape (StepState) and the client verbs:
//
//	GET  /debug/pause            the current pause state (paused/running/exited)
//	POST /debug/action           deliver a step action to a paused program
//	POST /debug/break            set/delete a breakpoint ({"spec":..,"delete":bool})
//
// Auth: when Token is non-empty, every request must carry
// `Authorization: Bearer <token>` (a static token, or a vault capability id
// — the broker validates it the same way the vault proxy does today). The
// loopback default + explicit AllowPublic mirror the vault proxy's hygiene.
package debugserve

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/native/help"
)

// Server exposes a registry's debug introspection over HTTP.
type Server struct {
	reg   *native.Registry
	token string // when non-empty, a required Bearer token

	mu     sync.Mutex
	events map[string][]Event // serverless channel: invocation id -> events
	maxEv  int                // per-invocation ring cap
}

// Event is one serverless-channel debug event (a tap/label/assert emission).
type Event struct {
	Seq     int    `json:"seq"`
	Label   string `json:"label"`
	Value   string `json:"value"`
	UnixSec int64  `json:"unix_sec"`
}

// NewServer wraps reg. token (optional) gates every request via Bearer auth.
func NewServer(reg *native.Registry, token string) *Server {
	return &Server{
		reg:    reg,
		token:  token,
		events: make(map[string][]Event),
		maxEv:  256,
	}
}

// Handler returns the HTTP handler (exposed so tests can mount it on an
// httptest.Server without binding a real socket).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/healthz", s.auth(s.handleHealthz))
	mux.HandleFunc("/debug/words", s.auth(s.handleWords))
	mux.HandleFunc("/debug/defs", s.auth(s.handleDefs))
	mux.HandleFunc("/debug/heap", s.auth(s.handleHeap))
	mux.HandleFunc("/debug/eval", s.auth(s.handleEval))
	mux.HandleFunc("/debug/emit", s.auth(s.handleEmit))
	mux.HandleFunc("/debug/events", s.auth(s.handleEvents))
	return mux
}

// ListenAndServe binds bind ("host:port") and serves until ctx is canceled.
// A non-loopback bind requires allowPublic, mirroring the vault proxy's
// guard against accidentally exposing introspection to the network.
func (s *Server) ListenAndServe(ctx context.Context, bind string, allowPublic bool) error {
	return s.ListenAndServeHandler(ctx, bind, allowPublic, s.Handler())
}

// ListenAndServeHandler is ListenAndServe with a caller-composed handler —
// the seam `boru debug serve --step` uses to mount the remote stepping
// routes alongside (and, for /debug/eval, in front of) the introspection
// set. The bind hygiene is identical.
func (s *Server) ListenAndServeHandler(ctx context.Context, bind string, allowPublic bool, h http.Handler) error {
	if !allowPublic && !isLoopback(bind) {
		return fmt.Errorf("debugserve: refusing non-loopback bind %q without allowPublic", bind)
	}
	ln, err := netListen("tcp", bind)
	if err != nil {
		return fmt.Errorf("debugserve: listen %s: %w", bind, err)
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Auth wraps a handler with the server's optional Bearer-token check —
// exported so externally-mounted routes (the stepping surface) share the
// exact gate the introspection routes use.
func (s *Server) Auth(h http.HandlerFunc) http.HandlerFunc { return s.auth(h) }

// auth wraps a handler with the optional Bearer-token check.
func (s *Server) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" && r.Header.Get("Authorization") != "Bearer "+s.token {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		h(w, r)
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleWords(w http.ResponseWriter, _ *http.Request) {
	words := append([]string(nil), help.Words()...)
	sort.Strings(words)
	writeJSON(w, http.StatusOK, map[string]any{"words": words})
}

func (s *Server) handleDefs(w http.ResponseWriter, _ *http.Request) {
	names := append([]string(nil), s.reg.Defs.Names()...)
	sort.Strings(names)
	defs := make(map[string]string, len(names))
	for _, n := range names {
		if v, ok := s.reg.Defs.Top(n); ok {
			defs[n] = native.FormatForPrint(v)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"defs": defs})
}

func (s *Server) handleHeap(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, ReadHeap())
}

func (s *Server) handleEval(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	result, evalErr := s.eval(string(body))
	if evalErr != nil {
		writeJSON(w, http.StatusOK, map[string]any{"error": evalErr.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

// eval runs src on the attached registry and returns the rendered
// top-of-stack result. Runs in a sub-engine so the attached registry's
// own state is observed (defs etc.) without a fresh registry.
func (s *Server) eval(src string) (string, error) {
	if s.reg.ParseFunc == nil {
		return "", fmt.Errorf("parser not configured")
	}
	tokens, perr := s.reg.ParseFunc(src)
	if perr != nil {
		return "", fmt.Errorf("parse: %w", perr)
	}
	res, rerr := native.New(s.reg).Run(tokens)
	if rerr != nil {
		return "", rerr
	}
	if len(res) == 0 {
		return native.FormatForPrint(native.NewNone()), nil
	}
	return native.FormatForPrint(res[len(res)-1]), nil
}

func (s *Server) handleEmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "missing id")
		return
	}
	var ev Event
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&ev); err != nil {
		writeErr(w, http.StatusBadRequest, "decode: "+err.Error())
		return
	}
	s.mu.Lock()
	buf := s.events[id]
	ev.Seq = len(buf)
	buf = append(buf, ev)
	if len(buf) > s.maxEv { // ring: drop oldest
		buf = buf[len(buf)-s.maxEv:]
	}
	s.events[id] = buf
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "missing id")
		return
	}
	s.mu.Lock()
	evs := append([]Event(nil), s.events[id]...)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"events": evs})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func isLoopback(bind string) bool {
	host, _, err := net.SplitHostPort(bind)
	if err != nil {
		host = bind
	}
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
