package modules

func init() {
	registerDocs("aql:cli", map[string]string{
		"parse":     "Parse an argument vector against a spec map; {ok:true flags args command} or {ok:false err usage}.",
		"dispatch":  "Decide what a program should do with its argv: {action:run|help|version|error, code, out, err, flags, args}.",
		"usage":     "Render a spec's help text; {width} wraps and {color} enables the palette.",
		"term-opts": "Turn what you know about the terminal (COLUMNS, NO_COLOR, is-tty) into {width, color} for Cli.usage.",
		"main":      "The imperative shell over dispatch: print, then exit or call the handler with {flags, args}.",
	})
}
