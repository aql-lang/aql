package modules

func init() {
	registerDocs("aql:net", map[string]string{
		"direct":  "Build and send a request against an API descriptor.",
		"fetch":   "Perform an HTTP request.",
		"prepare": "Build a request from an API descriptor without sending.",
	})
}
