package modules

func init() {
	registerDocs("boru:model", map[string]string{
		"new": "Construct a system model from a spec Map: `Model.new {src:'…'}` (inline " +
			".jsonic) or `{path:'model.jsonic', base:'model'}` (file). Other spec keys: `args` " +
			"(Map), `dryrun` (Boolean), `order` (List of action names), `watch` ({mod,add,rem}), " +
			"and `actions` (a Map of name → Function, or name → {run:Function, step:'pre'|'post'|'all'}). " +
			"Returns a Model value. Wraps github.com/voxgig/model (Go); the model is resolved via " +
			"aontu unification. All file I/O goes through the boru:io FileOps capability, so an " +
			"in-memory FileOps (tests) runs the whole model with no disk access.",
		"run": "Build the model once: `Model.run <Model>` resolves the .jsonic source into one " +
			"unified model (via aontu), runs the registered actions, and returns a result Map " +
			"{ok:Boolean, errs:[String], runlog:[String], model:Map}. Each action Function receives " +
			"the unified model Map; a Boolean result sets ok, a {ok,reload} Map supplies both.",
		"start": "Build once, then watch the source and rebuild on change: `Model.start <Model>` " +
			"returns the initial result Map (same shape as Model.run) and begins watching in the " +
			"background until Model.stop. Watch rebuilds run their BORU actions on an isolated fork " +
			"of the registry (taken at start time), so they never race the foreground interpreter.",
		"stop": "Stop watching a model started with Model.start: `Model.stop <Model>`.",
		"model": "The unified model Map after a build: `Model.model <Model>`. Errors if the model " +
			"has not been built yet (call Model.run or Model.start first).",
	})

	// The lifecycle is the API here: define, instantiate, run or
	// start/stop. Which word to reach for is not visible in signatures
	// that all take a Map.
	registerExamples("boru:model", map[string][]string{
		"model": {
			`def m (Model.model {                             ;# define the shape`,
			`  state: {count: 0}`,
			`  update: ([s:Map e:Map] => [ … ])`,
			`})`,
		},
		"new":   {`def inst (Model.new m {count: 5})                ;# an instance with initial state`},
		"run":   {`Model.run inst events                            ;# drive it to completion, return the final state`},
		"start": {`Model.start inst                                 ;# run it in the background instead`},
		"stop":  {`Model.stop inst                                  ;# ends a started instance`},
	})
}
