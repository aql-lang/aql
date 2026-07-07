package test

import (
	"os"
	"strings"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/capabilities"
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
)

// End-to-end tests for the networking verification apps written in AQL
// (design/examples/apps/): the todo REST API, the mini-redis server, and
// the streaming/resumable mini-S3. Each test loads the example file as a
// FILE MODULE (exercising the file-module loader too) and drives it over
// a real loopback socket on an ephemeral port.

// runAppSteps loads the named example app into an in-memory FS at
// /apps/<name> and runs the steps on a fully-wired registry (module
// resolver + parser + fileops).
func runAppSteps(t *testing.T, appFiles []string, steps []string) ([]native.Value, error) {
	t.Helper()
	mem := capabilities.NewMem()
	for _, name := range appFiles {
		src, err := os.ReadFile("../../../design/examples/apps/" + name)
		if err != nil {
			t.Fatalf("read app %s: %v", name, err)
		}
		mem.Files["/apps/"+name] = src
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.ParseFunc = parser.Parse
	native.SetHostFileOps(reg, mem)
	modules.InstallResolver(reg)
	engine := native.NewTop(reg)
	var result []native.Value
	for _, step := range steps {
		vals, pErr := parser.Parse(step)
		if pErr != nil {
			return nil, pErr
		}
		result, err = engine.Run(vals)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func appLastString(t *testing.T, vals []native.Value) string {
	t.Helper()
	if len(vals) == 0 {
		t.Fatal("no result")
	}
	s, err := eng.AsString(vals[len(vals)-1])
	if err != nil {
		t.Fatalf("expected String result, got %v", vals)
	}
	return s
}

// The todo REST API (todo-api.aql): CRUD over the http codec with :id
// route params, JSON bodies, tombstone deletes, and the request-count
// wrap middleware — the NETWORK-SERVERS.0.md §6.4 shape end to end.
func TestAppTodoAPI(t *testing.T) {
	out, err := runAppSteps(t, []string{"todo-api.aql"}, []string{
		`import "/apps/todo-api.aql"`,
		`import "aql:net"`,
		`def ln (TodoApi.serve {port: 0})`,
		`def lna (Net.addr ln)`,
		`def port lna.port`,
		`def ep (Net.connect {tcp: (join "" ["127.0.0.1:" (convert String port)]) codec: Net.http})`,
		// create
		`def c1 (call {method:"POST" path:"/todos" headers:{content-type:"application/json"} body:"{\"text\": \"buy milk\"}"} ep {timeout: 5000})`,
		// create without a body → 400
		`def c2 (call {method:"POST" path:"/todos"} ep {timeout: 5000})`,
		// read one (route param), update, list, delete, re-read (404), stats
		`def g1 (call {method:"GET" path:"/todos/1"} ep {timeout: 5000})`,
		`def m1 (call {method:"PUT" path:"/todos/1" headers:{content-type:"application/json"} body:"{\"done\": true}"} ep {timeout: 5000})`,
		`def l1 (call {method:"GET" path:"/todos"} ep {timeout: 5000})`,
		`def d1 (call {method:"DELETE" path:"/todos/1"} ep {timeout: 5000})`,
		`def g2 (call {method:"GET" path:"/todos/1"} ep {timeout: 5000})`,
		`def s1 (call {method:"GET" path:"/stats"} ep {timeout: 5000})`,
		`Net.close ep;`,
		`Net.close ln;`,
		`join "|" [(convert String c1.status) (convert String c2.status) (convert String g1.body)
		           (convert String m1.body) (convert String l1.body) (convert String d1.status)
		           (convert String g2.status) (convert String s1.body)]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := appLastString(t, out)
	parts := strings.Split(got, "|")
	if len(parts) != 8 {
		t.Fatalf("expected 8 fields, got %q", got)
	}
	if parts[0] != "201" {
		t.Errorf("POST status = %s, want 201", parts[0])
	}
	if parts[1] != "400" {
		t.Errorf("bodyless POST status = %s, want 400", parts[1])
	}
	if !strings.Contains(parts[2], `"buy milk"`) || !strings.Contains(parts[2], `"done":false`) {
		t.Errorf("GET one = %s", parts[2])
	}
	if !strings.Contains(parts[3], `"done":true`) {
		t.Errorf("PUT reply = %s, want done:true", parts[3])
	}
	if !strings.Contains(parts[4], `"buy milk"`) {
		t.Errorf("list = %s", parts[4])
	}
	if parts[5] != "204" {
		t.Errorf("DELETE status = %s, want 204", parts[5])
	}
	if parts[6] != "404" {
		t.Errorf("GET after delete = %s, want 404", parts[6])
	}
	if !strings.Contains(parts[7], "hits") {
		t.Errorf("stats = %s, want a hits count from the wrap middleware", parts[7])
	}
}
