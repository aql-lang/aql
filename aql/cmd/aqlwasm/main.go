//go:build js && wasm

// Command aqlwasm compiles the AQL engine to WebAssembly.
// Build: GOOS=js GOARCH=wasm go build -o aql.wasm ./cmd/aqlwasm
package main

import (
	"fmt"
	"strings"
	"syscall/js"

	aql "github.com/metsitaba/voxgig-exp/aql"
)

func main() {
	instance, err := aql.New()
	if err != nil {
		js.Global().Get("console").Call("error", "aql init failed: "+err.Error())
		return
	}
	instance.SetFileOps(aql.NewMemFileOps())

	js.Global().Set("aqlEval", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 {
			return map[string]any{"error": "missing code argument"}
		}
		code := args[0].String()

		result, err := instance.Run(code)
		if err != nil {
			return map[string]any{"error": err.Error()}
		}

		parts := make([]string, len(result))
		for i, v := range result {
			parts[i] = fmt.Sprintf("%v", v)
		}
		return map[string]any{"result": strings.Join(parts, " ")}
	}))

	// Signal that the WASM module is ready.
	if cb := js.Global().Get("__aqlReady"); !cb.IsUndefined() && !cb.IsNull() {
		cb.Invoke()
	}

	// Block forever so the Go runtime stays alive.
	select {}
}
