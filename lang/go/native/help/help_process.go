package help

// Help entries for the BEAM-style process words (design/PROCESSES.0.md)
// and the in-process service model (design/SERVICES.0.md). The category
// listing lives in help_categories.go ("concurrent").

func init() {
	register(&Entry{
		Word:    "spawn",
		Summary: "Start a lightweight process running a body; returns its Pid.",
		Description: "`spawn [body]` runs the quoted body on its own isolated process (a goroutine with a " +
			"forked registry) and returns an opaque `Pid` handle. The process owns a bounded mailbox " +
			"(default 1024 messages); `spawn [body] {mailbox: N, overflow: \"block\"|\"fail\"|\"drop\"}` " +
			"tunes it (mailbox: 0 = unbounded). A crash terminates only that process. Gated by the " +
			"`process` capability scope.",
		Examples: []string{
			`def pid (spawn [ receive [ {ping: 1 reply: Pid} [ send {pong: 1} reply ] after 5000 [ 0 ] ] ])`,
		},
	})

	register(&Entry{
		Word:    "self",
		Summary: "The current process's Pid.",
		Description: "`self` returns the Pid of the process the current code runs as. At the top level it " +
			"lazily creates the implicit main process, so a program can hand `self` to actors it spawns " +
			"and `receive` their replies. Parenthesise it in argument position: `send {reply: (self)} pid` " +
			"or inside a map literal `{reply: self}` (map values evaluate).",
		Examples: []string{
			`(typeof self) eq Pid ; # => true`,
		},
	})

	register(&Entry{
		Word:    "send",
		Summary: "Asynchronously deliver a message to a process or service.",
		Description: "`send <msg> <target>` enqueues msg into the target's mailbox and returns nothing. The " +
			"target is a `Pid`, a registered process name (String/Atom), or a `Service` (same dispatch as " +
			"`call`, reply discarded). Messages must be immutable — stateful containers (Store/Object/" +
			"Table/Flex) raise `not_sendable`; plain List/Map payloads are deep-copied at the boundary. " +
			"Sends to dead or unknown targets are silently dropped (fire-and-forget). A full \"fail\"-policy " +
			"mailbox raises `overload`.",
		Examples: []string{
			`send {cmd:"inc"} "counter"`,
		},
	})

	register(&Entry{
		Word:    "receive",
		Summary: "Take the front mailbox message and dispatch it by pattern clause.",
		Description: "`receive [ {pat} [body] … (after <ms> [body]) ]` blocks for the oldest mailbox message " +
			"and dispatches it: concrete-Scalar pattern fields are patrun routing tags (most specific wins); " +
			"type-named fields (`reply: Pid`, `n: Integer`) are binding slots — type-checked against the " +
			"message and bound into the clause body. `{}` is a catch-all. A message matching no clause " +
			"raises `no_match` (it is not saved). The optional `after <ms> [body]` clause fires when the " +
			"mailbox stays empty past the deadline; `after 0` polls.",
		Examples: []string{
			`receive [{tag:"count" count:Integer} [count] after 1000 [-1]]`,
		},
	})

	register(&Entry{
		Word:    "register",
		Summary: "Bind a name to a Pid in the shared process registry.",
		Description: "`register <name> <pid>` makes the process addressable by name: `send {…} \"name\"`. " +
			"Re-registering a name rebinds it. The binding is removed automatically when the process " +
			"exits. Returns nothing.",
		Examples: []string{
			`def pid (spawn [ receive [ {stop: 1} [ 0 ] after 5000 [ 0 ] ] ])  register "worker" pid`,
		},
	})

	register(&Entry{
		Word:    "whereis",
		Summary: "Look up a registered process name.",
		Description: "`whereis <name>` returns the Pid registered under name, or None when the name is " +
			"unknown (or its process has exited).",
		Examples: []string{
			`(whereis "ghost") eq None ; # => true`,
		},
	})

	register(&Entry{
		Word:    "unregister",
		Summary: "Remove a process name binding.",
		Description: "`unregister <name>` removes the name from the process registry (the process keeps " +
			"running). Unknown names are a no-op. Returns nothing.",
		Examples: []string{
			`unregister "worker"`,
		},
	})

	register(&Entry{
		Word:    "service",
		Summary: "Create a Service — private state plus pattern-matched request handlers.",
		Description: "`service {state}` returns a Service that owns a private mutable state (a flex map " +
			"seeded from the argument) and answers requests routed through a patrun of handlers. Register " +
			"handlers with `add {pattern} [handler] svc`, dispatch with `call`/`send`, decorate with " +
			"`wrap`. Dispatch is serialized — one request at a time — so handlers mutate state without " +
			"locks, and one service is safe to share across processes.",
		Examples: []string{
			`def svc (service {count:0})`,
		},
	})

	register(&Entry{
		Word:    "call",
		Summary: "Synchronous request → reply against a Service.",
		Description: "`call {request} svc` routes the request through the service's patrun (most-specific " +
			"pattern wins), runs the matched handler chain with `(request, state)`, and returns the " +
			"handler's reply. No matching handler raises `no_match` (unless a catch-all `add {} …` " +
			"exists). On a `connect`ed Endpoint the same word dispatches to the remote peer; " +
			"`call {req} ep {timeout: ms}` bounds the wait.",
		Examples: []string{
			`call {op:"inc"} svc`,
		},
	})

	register(&Entry{
		Word:    "state-of",
		Summary: "A Service's private state.",
		Description: "`state-of svc` returns the service's state (a flex map). State is normally touched " +
			"only inside handlers, which receive it as their second parameter; this accessor exists for " +
			"tests and diagnostics. (Named `state-of` rather than `state` so handler parameters can keep " +
			"the conventional name `state`.)",
		Examples: []string{
			`(state-of svc).count`,
		},
	})

	register(&Entry{
		Word:    "wrap",
		Summary: "Ambient middleware around every request into a Service.",
		Description: "`wrap [handler] svc` layers a `[req state prior]` handler around the whole dispatch — " +
			"whatever pattern matches — for cross-cutting concerns (auth, logging, body parsing). The " +
			"handler calls `prior req` to run the rest of the pipeline (possibly with a modified request), " +
			"may transform the reply, or may short-circuit by not calling `prior`. Multiple wraps nest, " +
			"newest outermost. Per-pattern layering (the `prior` stack) is `add` on an existing pattern; " +
			"`wrap` is the every-request axis.",
		Examples: []string{
			`wrap ([req:Map state:Any prior:Any] => [prior req]) svc`,
		},
	})
}
