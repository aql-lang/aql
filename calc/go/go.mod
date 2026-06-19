module github.com/aql-lang/aql/calc/go

go 1.24.7

require github.com/aql-lang/aql/eng/go v0.0.0

require (
	github.com/cockroachdb/apd/v3 v3.2.3 // indirect
	github.com/tabnas/json/go v0.2.0 // indirect
	github.com/tabnas/jsonic/go v0.2.0 // indirect
	github.com/tabnas/parser/go v0.2.0 // indirect
)

replace github.com/aql-lang/aql/eng/go => ../../eng/go
