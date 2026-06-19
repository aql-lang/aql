package modules

func init() {
	registerDocs("aql:keyval", map[string]string{
		"open":  "Open a key-value store (memory by default; file/redis/mongo/s3).",
		"get":   "Read the value for a key, or None if absent.",
		"set":   "Store a value under a key; returns the store for chaining.",
		"del":   "Delete a key; returns whether it was present.",
		"has":   "Report whether a key is present.",
		"keys":  "List stored keys, optionally filtered by prefix.",
		"close": "Close the store.",
	})
}
