// routes.go wires the api HTTP handler. Routes are documented in
// openapi.yaml; this file is the implementation.

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/aql-lang/aql/cmd/go/internal/service"
)

// nameRE matches a service name as specified in the OpenAPI spec.
var nameRE = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// handler returns the HTTP mux for the api server. Auth wraps every
// route except /healthz and /openapi.yaml.
func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()

	// Public routes.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(openAPISpec)
	})

	// Authenticated routes.
	mux.Handle("/v1/server", s.auth(http.HandlerFunc(s.handleServer)))
	mux.Handle("/v1/services", s.auth(http.HandlerFunc(s.handleServicesList)))
	// /v1/services/{name} and /v1/services/{name}/actions share a prefix;
	// dispatch on the trailing path component.
	mux.Handle("/v1/services/", s.auth(http.HandlerFunc(s.handleServiceByName)))

	return mux
}

// auth wraps a handler with bearer-token enforcement. With no token
// configured, requests pass through unauthenticated.
func (s *Server) auth(next http.Handler) http.Handler {
	if s.token == "" {
		return next
	}
	expected := "Bearer " + s.token
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != expected {
			writeError(w, http.StatusUnauthorized, "missing or invalid bearer token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleServer implements GET /v1/server.
func (s *Server) handleServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pid":           os.Getpid(),
		"version":       supervisorVersion,
		"startTime":     s.startTime.UTC().Format(time.RFC3339),
		"uptimeSeconds": time.Since(s.startTime).Seconds(),
		"serviceCount":  len(s.insp.Services()),
	})
}

// handleServicesList implements GET /v1/services.
func (s *Server) handleServicesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	svcs := s.insp.Services()
	out := make([]map[string]any, 0, len(svcs))
	for _, svc := range svcs {
		out = append(out, serviceEntity(svc))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleServiceByName routes /v1/services/{name} and
// /v1/services/{name}/actions.
func (s *Server) handleServiceByName(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/services/")
	if rest == "" {
		writeError(w, http.StatusNotFound, "no service in path")
		return
	}

	// Split into name and optional trailing segment.
	var name, tail string
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		name, tail = rest[:i], rest[i+1:]
	} else {
		name = rest
	}

	if !nameRE.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid service name")
		return
	}
	svc, ok := s.insp.ByName(name)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown service: "+name)
		return
	}

	switch tail {
	case "":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, http.StatusOK, serviceEntity(svc))
	case "actions":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		s.handleAction(w, r, svc)
	default:
		writeError(w, http.StatusNotFound, "unknown subresource: "+tail)
	}
}

// handleAction implements POST /v1/services/{name}/actions.
func (s *Server) handleAction(w http.ResponseWriter, r *http.Request, svc service.Service) {
	var body struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	switch body.Action {
	case "pause":
		p, ok := svc.(service.Pausable)
		if !ok {
			writeError(w, http.StatusConflict, svc.Name()+" does not support pause")
			return
		}
		if err := p.Pause(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	case "resume":
		p, ok := svc.(service.Pausable)
		if !ok {
			writeError(w, http.StatusConflict, svc.Name()+" does not support resume")
			return
		}
		if err := p.Resume(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	case "stop":
		// Stopping the api itself would deadlock: srv.Shutdown waits
		// for in-flight requests to return, but this handler IS the
		// in-flight request. Dispatch async after the response.
		if svc.Name() == "api" {
			writeJSON(w, http.StatusOK, serviceEntity(svc))
			go func() {
				asyncCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = s.insp.StopService(asyncCtx, svc.Name())
			}()
			return
		}
		if err := s.insp.StopService(ctx, svc.Name()); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "unknown action: "+body.Action)
		return
	}

	writeJSON(w, http.StatusOK, serviceEntity(svc))
}

// serviceEntity is the JSON shape returned for a Service. Matches
// the Service schema in openapi.yaml.
func serviceEntity(svc service.Service) map[string]any {
	_, pausable := svc.(service.Pausable)
	usesStdio := false
	if u, ok := svc.(service.StdioUser); ok {
		usesStdio = u.UsesStdio()
	}
	metadata := map[string]string{}
	if m, ok := svc.(service.WithMetadata); ok {
		metadata = m.Metadata()
	}
	return map[string]any{
		"name":      svc.Name(),
		"state":     svc.Status().String(),
		"pausable":  pausable,
		"usesStdio": usesStdio,
		"metadata":  metadata,
	}
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

// contextWithTimeout returns a context derived from the request's
// context, bounded by the given timeout. Used for handler actions
// that delegate to service methods.
func contextWithTimeout(r *http.Request, d time.Duration) (ctx context.Context, cancel context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}
