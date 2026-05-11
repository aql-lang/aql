// Command solardemo runs a minimal HTTP server implementing the Solar System API
// used for testing AQL API operations against a real server.
//
// Usage (from the cmd/go directory):
//
//	go run ./solardemo
//
// The server listens on :8901 and provides CRUD endpoints for planets and moons.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
)

type store struct {
	mu      sync.RWMutex
	planets map[string]map[string]any
	moons   map[string]map[string]any
	nextID  int
}

func newStore() *store {
	s := &store{
		planets: map[string]map[string]any{},
		moons:   map[string]map[string]any{},
		nextID:  1,
	}
	// Seed with initial data.
	s.planets["planet01"] = map[string]any{
		"id": "planet01", "name": "Mercury", "kind": "terrestrial", "diameter": 4879,
	}
	s.planets["planet02"] = map[string]any{
		"id": "planet02", "name": "Venus", "kind": "terrestrial", "diameter": 12104,
	}
	s.planets["planet03"] = map[string]any{
		"id": "planet03", "name": "Earth", "kind": "terrestrial", "diameter": 12756,
	}
	s.moons["moon01"] = map[string]any{
		"id": "moon01", "name": "Luna", "kind": "natural", "diameter": 3474, "planet_id": "planet03",
	}
	return s
}

func (s *store) genID() string {
	s.nextID++
	return fmt.Sprintf("gen%04d", s.nextID-1)
}

func main() {
	s := newStore()

	http.HandleFunc("/api/planet", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case "GET":
			// List planets.
			list := make([]any, 0, len(s.planets))
			for _, p := range s.planets {
				list = append(list, p)
			}
			json.NewEncoder(w).Encode(list)
		case "POST":
			// Create planet.
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"error":"bad json"}`, 400)
				return
			}
			id, _ := body["id"].(string)
			if id == "" {
				id = s.genID()
				body["id"] = id
			}
			s.planets[id] = body
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(body)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/api/planet/", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")

		// Parse path: /api/planet/{id} or /api/planet/{planet_id}/moon[/{moon_id}]
		path := strings.TrimPrefix(r.URL.Path, "/api/planet/")
		parts := strings.Split(path, "/")

		if len(parts) >= 2 && parts[1] == "moon" {
			// Moon endpoints.
			planetID := parts[0]
			if len(parts) == 2 {
				// /api/planet/{planet_id}/moon
				switch r.Method {
				case "GET":
					list := make([]any, 0)
					for _, m := range s.moons {
						if m["planet_id"] == planetID {
							list = append(list, m)
						}
					}
					json.NewEncoder(w).Encode(list)
				case "POST":
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						http.Error(w, `{"error":"bad json"}`, 400)
						return
					}
					id, _ := body["id"].(string)
					if id == "" {
						id = s.genID()
						body["id"] = id
					}
					body["planet_id"] = planetID
					s.moons[id] = body
					json.NewEncoder(w).Encode(body)
				default:
					http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
				}
			} else if len(parts) == 3 {
				// /api/planet/{planet_id}/moon/{moon_id}
				moonID := parts[2]
				moon, ok := s.moons[moonID]
				if !ok {
					http.Error(w, `{"error":"not found"}`, 404)
					return
				}
				switch r.Method {
				case "GET":
					json.NewEncoder(w).Encode(moon)
				case "PUT":
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						http.Error(w, `{"error":"bad json"}`, 400)
						return
					}
					for k, v := range body {
						moon[k] = v
					}
					json.NewEncoder(w).Encode(moon)
				case "DELETE":
					delete(s.moons, moonID)
					json.NewEncoder(w).Encode(map[string]any{"ok": true})
				default:
					http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
				}
			}
			return
		}

		// Planet endpoints: /api/planet/{id}
		if len(parts) != 1 || parts[0] == "" {
			http.Error(w, `{"error":"not found"}`, 404)
			return
		}
		planetID := parts[0]

		switch r.Method {
		case "GET":
			planet, ok := s.planets[planetID]
			if !ok {
				http.Error(w, `{"error":"not found"}`, 404)
				return
			}
			json.NewEncoder(w).Encode(planet)
		case "PUT":
			planet, ok := s.planets[planetID]
			if !ok {
				http.Error(w, `{"error":"not found"}`, 404)
				return
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"error":"bad json"}`, 400)
				return
			}
			for k, v := range body {
				planet[k] = v
			}
			json.NewEncoder(w).Encode(planet)
		case "DELETE":
			if _, ok := s.planets[planetID]; !ok {
				http.Error(w, `{"error":"not found"}`, 404)
				return
			}
			delete(s.planets, planetID)
			json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	log.Println("solardemo server listening on :8901")
	log.Fatal(http.ListenAndServe(":8901", nil))
}
