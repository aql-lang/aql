package modules

func init() {
	registerDocs("aql:vault-tui", map[string]string{
		"app": "The vault TUI's {init update view} app map — run it under Tui.run, or embed it in a harness.",
		"run": "Run the vault TUI against the registered terminal and vault backends; blocks until quit, returns the final state.",
	})

	// Both words hand back the standard model/update/view triple, which is
	// the only thing worth knowing and is invisible in a Map return type.
	registerExamples("aql:vault-tui", map[string][]string{
		"app": {
			`join "," (keys (VaultTui.app {}))                ;# 'init,update,view'`,
			`;# So it can be embedded in a larger Tui.run, or extended.`,
		},
		"run": {`VaultTui.run {}                                  ;# the vault manager as a full-screen app`},
	})
}
