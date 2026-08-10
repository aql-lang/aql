module github.com/boru-lang/boru/parser/go

go 1.24.7

require (
	github.com/boru-lang/boru/core/go v0.0.0
	github.com/cockroachdb/apd/v3 v3.2.3
	github.com/tabnas/jsonic/go v0.6.1
)

require (
	github.com/tabnas/json/go v0.5.1 // indirect
	github.com/tabnas/parser/go v0.8.1 // indirect
)

replace github.com/boru-lang/boru/core/go => ../../core/go
