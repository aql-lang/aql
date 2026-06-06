module github.com/aql-lang/aql/calc/go

go 1.24.7

require github.com/aql-lang/aql/eng/go v0.0.0

require (
	github.com/cockroachdb/apd/v3 v3.2.3 // indirect
	github.com/jsonicjs/jsonic/go v0.1.6 // indirect
)

replace github.com/aql-lang/aql/eng/go => ../../eng/go
