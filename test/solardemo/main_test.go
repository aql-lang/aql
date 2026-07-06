package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// doJSON drives one request against the production mux and decodes the
// JSON body into out (which may be nil to skip decoding). Returns the
// status code.
func doJSON(t *testing.T, mux http.Handler, method, path, body string, out any) int {
	t.Helper()
	var rdr *strings.Reader
	if body == "" {
		rdr = strings.NewReader("")
	} else {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if out != nil && rec.Code == 200 {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("%s %s: bad JSON response %q: %v", method, path, rec.Body.String(), err)
		}
	}
	return rec.Code
}

func TestPlanetCRUD(t *testing.T) {
	mux := newMux(newStore())

	// List the seeded planets.
	var list []map[string]any
	if code := doJSON(t, mux, "GET", "/api/planet", "", &list); code != 200 {
		t.Fatalf("list planets: status %d", code)
	}
	if len(list) != 3 {
		t.Fatalf("seeded planet count = %d, want 3", len(list))
	}

	// Read one.
	var earth map[string]any
	if code := doJSON(t, mux, "GET", "/api/planet/planet03", "", &earth); code != 200 {
		t.Fatalf("get planet03: status %d", code)
	}
	if earth["name"] != "Earth" {
		t.Errorf("planet03 name = %v, want Earth", earth["name"])
	}

	// Create with explicit id, then with generated id.
	var created map[string]any
	if code := doJSON(t, mux, "POST", "/api/planet", `{"id":"planet09","name":"Pluto","kind":"dwarf"}`, &created); code != 200 {
		t.Fatalf("create planet: status %d", code)
	}
	if created["id"] != "planet09" {
		t.Errorf("created id = %v, want planet09", created["id"])
	}
	created = nil
	if code := doJSON(t, mux, "POST", "/api/planet", `{"name":"Nibiru"}`, &created); code != 200 {
		t.Fatalf("create planet (gen id): status %d", code)
	}
	if id, _ := created["id"].(string); !strings.HasPrefix(id, "gen") {
		t.Errorf("generated id = %v, want gen-prefixed", created["id"])
	}

	// Update.
	var updated map[string]any
	if code := doJSON(t, mux, "PUT", "/api/planet/planet09", `{"diameter":2377}`, &updated); code != 200 {
		t.Fatalf("update planet: status %d", code)
	}
	if updated["diameter"] != float64(2377) {
		t.Errorf("updated diameter = %v, want 2377", updated["diameter"])
	}

	// Delete, then confirm gone.
	if code := doJSON(t, mux, "DELETE", "/api/planet/planet09", "", nil); code != 200 {
		t.Fatalf("delete planet: status %d", code)
	}
	if code := doJSON(t, mux, "GET", "/api/planet/planet09", "", nil); code != 404 {
		t.Errorf("get deleted planet: status %d, want 404", code)
	}
}

func TestPlanetRejections(t *testing.T) {
	mux := newMux(newStore())
	cases := []struct {
		name         string
		method, path string
		body         string
		want         int
	}{
		{"bad json create", "POST", "/api/planet", `{not json`, 400},
		{"method not allowed on collection", "PATCH", "/api/planet", "", 405},
		{"get missing planet", "GET", "/api/planet/nope", "", 404},
		{"put missing planet", "PUT", "/api/planet/nope", `{"a":1}`, 404},
		{"delete missing planet", "DELETE", "/api/planet/nope", "", 404},
		{"bad json update", "PUT", "/api/planet/planet01", `{not json`, 400},
		{"method not allowed on item", "PATCH", "/api/planet/planet01", "", 405},
		{"empty id path", "GET", "/api/planet/", "", 404},
	}
	for _, c := range cases {
		if code := doJSON(t, mux, c.method, c.path, c.body, nil); code != c.want {
			t.Errorf("%s: status %d, want %d", c.name, code, c.want)
		}
	}
}

func TestMoonCRUD(t *testing.T) {
	mux := newMux(newStore())

	// List Earth's seeded moon.
	var list []map[string]any
	if code := doJSON(t, mux, "GET", "/api/planet/planet03/moon", "", &list); code != 200 {
		t.Fatalf("list moons: status %d", code)
	}
	if len(list) != 1 || list[0]["name"] != "Luna" {
		t.Fatalf("seeded moons = %v, want [Luna]", list)
	}
	// A moonless planet lists empty (not an error).
	list = nil
	if code := doJSON(t, mux, "GET", "/api/planet/planet01/moon", "", &list); code != 200 {
		t.Fatalf("list moons of planet01: status %d", code)
	}
	if len(list) != 0 {
		t.Errorf("planet01 moons = %v, want none", list)
	}

	// Create (id generated, planet_id stamped).
	var moon map[string]any
	if code := doJSON(t, mux, "POST", "/api/planet/planet01/moon", `{"name":"Test"}`, &moon); code != 200 {
		t.Fatalf("create moon: status %d", code)
	}
	if moon["planet_id"] != "planet01" {
		t.Errorf("created moon planet_id = %v, want planet01", moon["planet_id"])
	}

	// Read / update / delete by id.
	var luna map[string]any
	if code := doJSON(t, mux, "GET", "/api/planet/planet03/moon/moon01", "", &luna); code != 200 {
		t.Fatalf("get moon01: status %d", code)
	}
	var upd map[string]any
	if code := doJSON(t, mux, "PUT", "/api/planet/planet03/moon/moon01", `{"diameter":9999}`, &upd); code != 200 {
		t.Fatalf("update moon01: status %d", code)
	}
	if upd["diameter"] != float64(9999) {
		t.Errorf("updated moon diameter = %v, want 9999", upd["diameter"])
	}
	if code := doJSON(t, mux, "DELETE", "/api/planet/planet03/moon/moon01", "", nil); code != 200 {
		t.Fatalf("delete moon01: status %d", code)
	}
	if code := doJSON(t, mux, "GET", "/api/planet/planet03/moon/moon01", "", nil); code != 404 {
		t.Errorf("get deleted moon: status %d, want 404", code)
	}
}

func TestMoonRejections(t *testing.T) {
	mux := newMux(newStore())
	cases := []struct {
		name         string
		method, path string
		body         string
		want         int
	}{
		{"bad json create", "POST", "/api/planet/planet03/moon", `{oops`, 400},
		{"method not allowed on collection", "PATCH", "/api/planet/planet03/moon", "", 405},
		{"get missing moon", "GET", "/api/planet/planet03/moon/nope", "", 404},
		{"bad json update", "PUT", "/api/planet/planet03/moon/moon01", `{oops`, 400},
		{"method not allowed on item", "PATCH", "/api/planet/planet03/moon/moon01", "", 405},
	}
	for _, c := range cases {
		if code := doJSON(t, mux, c.method, c.path, c.body, nil); code != c.want {
			t.Errorf("%s: status %d, want %d", c.name, code, c.want)
		}
	}
}

func TestGenIDSequence(t *testing.T) {
	s := newStore()
	if got := s.genID(); got != "gen0001" {
		t.Errorf("first genID = %q, want gen0001", got)
	}
	if got := s.genID(); got != "gen0002" {
		t.Errorf("second genID = %q, want gen0002", got)
	}
}

// main is covered through the seams (design/TEST-SEAMS.10.md).
func TestMainThroughSeams(t *testing.T) {
	prevExit, prevListen := osExit, listenServe
	t.Cleanup(func() { osExit, listenServe = prevExit, prevListen })

	// Clean serve return: main falls through without exiting.
	listenServe = func(addr string, h http.Handler) error {
		if addr != ":8901" || h == nil {
			t.Errorf("listenServe got addr %q, handler %v", addr, h)
		}
		return nil
	}
	osExit = func(int) { t.Error("osExit called on clean serve return") }
	main()

	// Serve failure exits 1.
	listenServe = func(string, http.Handler) error { return errors.New("bind failed") }
	var code int
	osExit = func(c int) { code = c }
	main()
	if code != 1 {
		t.Errorf("main with failing serve exited %d, want 1", code)
	}
}
