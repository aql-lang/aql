package modules

func init() {
	registerDocs("aql:io", map[string]string{
		"read":     "Read a file; {enc} decodes utf8/bytes/utf16le/utf16be/latin1.",
		"write":    "Write to a file, returning its path; {enc} encodes, {mode:'append'} appends.",
		"printstr": "Write formatted text to output, leaving no value.",
		"stdin":    "Standard-input handle.",
		"stdout":   "Standard-output handle.",
		"stderr":   "Standard-error handle.",
		"trace":    "Run a list as a traced sub-program, returning its result.",
		"folder":   "Create a directory, returning its Path.",
		"stat":     "Describe a path (name/size/type/mode/mtime/owner/group; {xattr:true} attaches attributes), or none if absent.",
		"list":     "List a directory's entries; {detail} for records, {recursive} to walk, {match} to glob-filter.",
		"remove":   "Delete a path; {recursive} for a tree, {force} to ignore absent.",
		"move":     "Rename/move a path, returning the destination; {overwrite:false} refuses an existing one.",
		"copy":     "Copy a path to a destination; {recursive} copies a tree, {overwrite:false} refuses an existing one.",
		"link":     "Link dst to src: a symlink by default, a hard link with {hard}.",
		"touch":    "Create a path if absent and set {mode}/{mtime}/{atime}/{size}/{owner}/{group}/{xattr}.",
		"watch":    "Run a body per change event on a path; returns a Watcher.",
		"unwatch":  "Stop a Watcher, closing its event stream.",
		"mount":    "Install a map of AQL handler fns as the filesystem (the FileOps bridge).",
		"unmount":  "Restore the filesystem that was active before mount.",
	})
}
