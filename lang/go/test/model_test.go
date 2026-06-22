package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

// The aql:model module is a thin AQL surface over the upstream Go
// implementation of @voxgig/model (github.com/voxgig/model): it unifies
// .jsonic source into one system model (via aontu) and runs AQL-function
// actions over it. These tests drive Model.new / run / model / actions
// through the live module.

const modelImp = `"aql:model" import end  `

// TestModelRunInline pins the build-once path over inline source: the result
// reports ok, and Model.model returns the unified model.
func TestModelRunInline(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	prog := modelImp + `def m (Model.new {src:'a: 1 b: 2'}) (Model.run m) get 'ok'`
	if got := fmt.Sprintf("%v", runLast(t, a, prog)); got != "true" {
		t.Fatalf("run ok: got %v, want true", got)
	}
	if got := fmt.Sprintf("%v", runLast(t, a, modelImp+`def m (Model.new {src:'a: 1 b: 2'}) Model.run m drop (Model.model m) get 'b'`)); got != "2" {
		t.Errorf("model b: got %v, want 2", got)
	}
}

// TestModelType pins that a Model value reports type Model.
func TestModelType(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	if got := fmt.Sprintf("%v", runLast(t, a, modelImp+`typeof (Model.new {src:'a: 1'})`)); got != "Model" {
		t.Errorf("typeof Model.new: got %v, want Model", got)
	}
}

// TestModelActionSeesModel pins the action bridge: an AQL-function action
// receives the unified model Map and its result controls the build ok flag.
func TestModelActionSeesModel(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	okProg := modelImp + `def m (Model.new {src:'a: 1', actions:{check:([mod:Any] => [{ok: ((mod get 'a') eq 1)}])}}) (Model.run m) get 'ok'`
	if got := fmt.Sprintf("%v", runLast(t, a, okProg)); got != "true" {
		t.Errorf("action ok: got %v, want true", got)
	}
	// An action returning false fails the build.
	badProg := modelImp + `def m (Model.new {src:'a: 1', actions:{bad:([mod:Any] => [false])}}) (Model.run m) get 'ok'`
	if got := fmt.Sprintf("%v", runLast(t, a, badProg)); got != "false" {
		t.Errorf("failing action: got %v, want false", got)
	}
}

// TestModelFilePath pins file-path source: a .jsonic file on disk resolves
// the same as inline source.
func TestModelFilePath(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "model.jsonic")
	if werr := os.WriteFile(file, []byte("svc: name: 'orders'"), 0o600); werr != nil {
		t.Fatalf("write model: %v", werr)
	}
	prog := fmt.Sprintf("%sdef m (Model.new {path:'%s', base:'%s', dryrun:true}) Model.run m drop ((Model.model m) get 'svc') get 'name'",
		modelImp, file, dir)
	if got := fmt.Sprintf("%v", runLast(t, a, prog)); got != "orders" {
		t.Errorf("file path model: got %v, want orders", got)
	}
}

// TestModelConflict pins that a unification conflict in the source surfaces
// as a failed build (ok:false) rather than a crash.
func TestModelConflict(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	prog := modelImp + `def m (Model.new {src:'a: 1 a: 2'}) (Model.run m) get 'ok'`
	if got := fmt.Sprintf("%v", runLast(t, a, prog)); got != "false" {
		t.Errorf("conflict ok: got %v, want false", got)
	}
}

// TestModelErrors pins the loud failures of the spec and word contract.
func TestModelErrors(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"no source", modelImp + `Model.new {}`, "model_bad_spec"},
		{"bad action", modelImp + `Model.new {src:'a:1', actions:{x:42}}`, "model_bad_action"},
		{"model before build", modelImp + `Model.model (Model.new {src:'a:1'})`, "model_not_built"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			if _, err := a.Run(c.src); err == nil {
				t.Fatalf("%s: expected error, got nil", c.name)
			} else if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("%s: error %q does not contain %q", c.name, err.Error(), c.want)
			}
		})
	}
}
