package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/aql-lang/aql/lang"
)

type evalRequest struct {
	Code string `json:"code"`
}

type evalResponse struct {
	Result []string `json:"result,omitempty"`
	Output string   `json:"output,omitempty"`
	Error  string   `json:"error,omitempty"`
}

func main() {
	port := flag.Int("port", 8080, "HTTP port")
	flag.Parse()

	instance, err := lang.New()
	if err != nil {
		log.Fatalf("failed to create AQL instance: %v", err)
	}

	var mu sync.Mutex
	var outBuf bytes.Buffer
	instance.SetOutput(&outBuf)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(indexHTML))
	})

	http.HandleFunc("/api/eval", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req evalRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, evalResponse{Error: "invalid request body"})
			return
		}

		mu.Lock()
		outBuf.Reset()
		result, err := instance.Run(req.Code)
		printed := outBuf.String()
		mu.Unlock()

		resp := evalResponse{Output: printed}
		if err != nil {
			resp.Error = err.Error()
		} else {
			resp.Result = make([]string, len(result))
			for i, v := range result {
				resp.Result[i] = fmt.Sprintf("%v", v)
			}
		}
		writeJSON(w, resp)
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("AQL Web REPL listening on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>AQL REPL</title>
<style>
  :root {
    --bg: #1e1e2e;
    --surface: #181825;
    --text: #cdd6f4;
    --subtext: #a6adc8;
    --green: #a6e3a1;
    --red: #f38ba8;
    --blue: #89b4fa;
    --mauve: #cba6f7;
    --border: #313244;
    --input-bg: #11111b;
  }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    background: var(--bg);
    color: var(--text);
    font-family: 'JetBrains Mono', 'Fira Code', 'Cascadia Code', 'Consolas', monospace;
    font-size: 14px;
    height: 100vh;
    display: flex;
    flex-direction: column;
  }
  header {
    padding: 12px 20px;
    border-bottom: 1px solid var(--border);
    display: flex;
    align-items: center;
    gap: 12px;
  }
  header h1 {
    font-size: 16px;
    font-weight: 600;
    color: var(--mauve);
  }
  header span {
    font-size: 12px;
    color: var(--subtext);
  }
  #output {
    flex: 1;
    overflow-y: auto;
    padding: 16px 20px;
    white-space: pre-wrap;
    word-break: break-all;
  }
  .line { margin-bottom: 4px; }
  .line-input { color: var(--blue); }
  .line-result { color: var(--green); }
  .line-error { color: var(--red); }
  .line-info { color: var(--subtext); font-style: italic; }
  .prompt { color: var(--mauve); user-select: none; }
  #input-row {
    display: flex;
    align-items: center;
    padding: 8px 20px 12px;
    border-top: 1px solid var(--border);
    background: var(--surface);
  }
  #input-row .prompt {
    color: var(--mauve);
    margin-right: 8px;
    font-size: 14px;
    flex-shrink: 0;
  }
  #input {
    flex: 1;
    background: var(--input-bg);
    border: 1px solid var(--border);
    border-radius: 4px;
    color: var(--text);
    font-family: inherit;
    font-size: 14px;
    padding: 8px 12px;
    outline: none;
  }
  #input:focus { border-color: var(--mauve); }
  .help-hint {
    padding: 4px 20px 12px;
    color: var(--subtext);
    font-size: 12px;
  }
</style>
</head>
<body>
<header>
  <h1>AQL REPL</h1>
  <span>concatenative query language</span>
</header>
<div id="output">
<div class="line line-info">Welcome to the AQL Web REPL. *Type expressions and press Enter to evaluate.</div>
<div class="line line-info">Examples: 1 add 2 &nbsp;|&nbsp; "hello" upper &nbsp;|&nbsp; [1 2 3] len &nbsp;|&nbsp; def double [dup add]</div>
</div>
<div id="input-row">
  <span class="prompt">&gt;&gt;</span>
  <input id="input" type="text" autofocus autocomplete="off" spellcheck="false" placeholder="Enter AQL expression...">
</div>
<script>
(function() {
  const output = document.getElementById('output');
  const input = document.getElementById('input');
  const history = [];
  let historyIdx = -1;

  function addLine(cls, text) {
    const div = document.createElement('div');
    div.className = 'line ' + cls;
    div.textContent = text;
    output.appendChild(div);
    output.scrollTop = output.scrollHeight;
  }

  async function evaluate(code) {
    addLine('line-input', '>> ' + code);
    try {
      const res = await fetch('/api/eval', {
        method: 'POST',
        headers: {'Content-*Type': 'application/json'},
        body: JSON.stringify({code: code})
      });
      const data = await res.json();
      if (data.error) {
        addLine('line-error', '  error: ' + data.error);
      } else if (data.result && data.result.length > 0) {
        addLine('line-result', data.result.join(' '));
      }
    } catch (e) {
      addLine('line-error', '  error: ' + e.message);
    }
  }

  input.addEventListener('keydown', function(e) {
    if (e.key === 'Enter') {
      const code = input.value.trim();
      if (!code) return;
      history.push(code);
      historyIdx = history.length;
      input.value = '';
      evaluate(code);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (historyIdx > 0) {
        historyIdx--;
        input.value = history[historyIdx];
      }
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (historyIdx < history.length - 1) {
        historyIdx++;
        input.value = history[historyIdx];
      } else {
        historyIdx = history.length;
        input.value = '';
      }
    }
  });
})();
</script>
</body>
</html>
`
