//go:build js && wasm

// Command wpg/wasm compiles the AQL engine to WebAssembly.
// Build (from the wpg directory): GOOS=js GOARCH=wasm go build -o aql.wasm ./wasm
package main

import (
	"bytes"
	"fmt"
	"strings"
	"syscall/js"

	"github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/formatter"
)

func main() {
	instance, err := lang.New()
	if err != nil {
		js.Global().Get("console").Call("error", "aql init failed: "+err.Error())
		return
	}
	instance.SetFileOps(lang.NewMemFileOps())

	var outBuf bytes.Buffer
	instance.SetOutput(&outBuf)

	js.Global().Set("aqlEval", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 {
			return map[string]any{"error": "missing code argument"}
		}
		code := args[0].String()

		outBuf.Reset()
		result, err := instance.Run(code)

		printed := outBuf.String()

		if err != nil {
			return map[string]any{"error": err.Error(), "output": printed}
		}

		parts := make([]string, len(result))
		for i, v := range result {
			parts[i] = fmt.Sprintf("%v", v)
		}
		return map[string]any{"result": strings.Join(parts, " "), "output": printed}
	}))

	js.Global().Set("aqlFmt", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 {
			return ""
		}
		return formatter.Format(args[0].String())
	}))

	// Signal that the WASM module is ready.
	if cb := js.Global().Get("__aqlReady"); !cb.IsUndefined() && !cb.IsNull() {
		cb.Invoke()
	}

	// Block forever so the Go runtime stays alive.
	select {}
}
