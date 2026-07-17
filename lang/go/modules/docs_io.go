package modules

func init() {
	registerDocs("aql:io", map[string]string{
		"read":     "Read the contents of a file.",
		"write":    "Write a string to a file, returning its path.",
		"printstr": "Write formatted text to output, leaving no value.",
		"stdin":    "Standard-input handle.",
		"stdout":   "Standard-output handle.",
		"stderr":   "Standard-error handle.",
		"trace":    "Run a list as a traced sub-program, returning its result.",
		"folder":   "Create a directory, returning its Path.",
		"stat":     "Describe a path (name/size/type/mode/mtime), or none if absent.",
		"list":     "List a directory's entries; {detail} for records, {recursive} to walk.",
		"remove":   "Delete a path; {recursive} for a tree, {force} to ignore absent.",
		"move":     "Rename/move a path to a new location, returning the destination.",
		"copy":     "Copy a path to a destination; {recursive} copies a directory tree.",
		"link":     "Link dst to src: a symlink by default, a hard link with {hard}.",
		"touch":    "Create a path if absent and set {mode}/{mtime}/{atime}/{size}.",
		"watch":    "Run a body per change event on a path; returns a Watcher.",
		"unwatch":  "Stop a Watcher, closing its event stream.",
	})
}
