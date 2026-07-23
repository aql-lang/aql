package modules

func init() {
	registerDocs("aql:vault-tui", map[string]string{
		"app": "The vault TUI's {init update view} app map — run it under Tui.run, or embed it in a harness.",
		"run": "Run the vault TUI against the registered terminal and vault backends; blocks until quit, returns the final state.",
	})
}
