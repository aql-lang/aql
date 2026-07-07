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

// The mini-redis server (mini-redis.aql): the common Redis commands
// over a custom AQL codec (inline commands in, RESP-flavoured lines
// out) — the NETWORK-SERVERS.0.md §6.6 custom-protocol story as a
// real app. The client side is plain Net.lines.
func TestAppMiniRedis(t *testing.T) {
	out, err := runAppSteps(t, []string{"mini-redis.aql"}, []string{
		`import "/apps/mini-redis.aql"`,
		`import "aql:net"`,
		`def ln (MiniRedis.serve {port: 0})`,
		`def lna (Net.addr ln)`,
		`def port lna.port`,
		`def ep (MiniRedis.connect (join "" ["127.0.0.1:" (convert String port)]))`,
		`def r-ping (MiniRedis.cmd ep "PING")`,
		`def r-set (MiniRedis.cmd ep "SET greeting hello world")`,
		`def r-get (MiniRedis.cmd ep "GET greeting")`,
		`def r-exists (MiniRedis.cmd ep "EXISTS greeting")`,
		`def r-incr1 (MiniRedis.cmd ep "INCR counter")`,
		`def r-incr2 (MiniRedis.cmd ep "INCR counter")`,
		`def r-keys (MiniRedis.cmd ep "KEYS *")`,
		`def r-del (MiniRedis.cmd ep "DEL greeting")`,
		`def r-get2 (MiniRedis.cmd ep "GET greeting")`,
		`def r-keys2 (MiniRedis.cmd ep "KEYS *")`,
		`MiniRedis.cmd ep "RPUSH mylist a" drop`,
		`MiniRedis.cmd ep "RPUSH mylist b" drop`,
		`def r-lpush (MiniRedis.cmd ep "LPUSH mylist z")`,
		`def r-lrange (MiniRedis.cmd ep "LRANGE mylist 0 -1")`,
		`def r-llen (MiniRedis.cmd ep "LLEN mylist")`,
		`MiniRedis.cmd ep "HSET h f1 v1" drop`,
		`def r-hget (MiniRedis.cmd ep "HGET h f1")`,
		`def r-hmiss (MiniRedis.cmd ep "HGET h nope")`,
		`def r-expire (MiniRedis.cmd ep "EXPIRE counter 100")`,
		`def r-ttl (MiniRedis.cmd ep "TTL counter")`,
		`def r-ttl-none (MiniRedis.cmd ep "TTL mylist")`,
		// lazy expiry: EXPIRE 0 lapses immediately
		`MiniRedis.cmd ep "EXPIRE counter 0" drop`,
		`def r-lapsed (MiniRedis.cmd ep "GET counter")`,
		`def r-bogus (MiniRedis.cmd ep "BOGUS x")`,
		`Net.close ep;`,
		`Net.close ln;`,
		`join "|" [r-ping r-set r-get r-exists r-incr1 r-incr2 r-keys r-del r-get2 r-keys2
		           r-lpush r-lrange r-llen r-hget r-hmiss r-expire r-ttl r-ttl-none r-bogus r-lapsed]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := appLastString(t, out)
	parts := strings.Split(got, "|")
	want := []struct {
		i    int
		name string
		val  string
	}{
		{0, "PING", "+PONG"},
		{1, "SET", "+OK"},
		{2, "GET", "hello world"},
		{3, "EXISTS", ":1"},
		{4, "INCR", ":1"},
		{5, "INCR again", ":2"},
		{7, "DEL", ":1"},
		{8, "GET after DEL", "$-1"},
		{9, "KEYS after DEL", "counter"},
		{10, "LPUSH", ":3"},
		{11, "LRANGE 0 -1", "z a b"},
		{12, "LLEN", ":3"},
		{13, "HGET", "v1"},
		{14, "HGET miss", "$-1"},
		{15, "EXPIRE", ":1"},
	}
	if len(parts) != 20 {
		t.Fatalf("expected 20 replies, got %d: %q", len(parts), got)
	}
	for _, w := range want {
		if parts[w.i] != w.val {
			t.Errorf("%s = %q, want %q", w.name, parts[w.i], w.val)
		}
	}
	if !strings.Contains(parts[6], "greeting") || !strings.Contains(parts[6], "counter") {
		t.Errorf("KEYS = %q, want both keys", parts[6])
	}
	if parts[16] != ":99" && parts[16] != ":100" {
		t.Errorf("TTL = %q, want :99 or :100 (seconds remaining)", parts[16])
	}
	if parts[17] != ":-2" {
		t.Errorf("TTL of a non-string key = %q, want :-2", parts[17])
	}
	if !strings.Contains(parts[18], "-ERR") || !strings.Contains(parts[18], "BOGUS") {
		t.Errorf("unknown command = %q, want -ERR naming it", parts[18])
	}
	if parts[19] != "$-1" {
		t.Errorf("GET after EXPIRE 0 = %q, want $-1 (lazy expiry)", parts[19])
	}
}
