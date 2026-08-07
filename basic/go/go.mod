module github.com/boru-lang/boru/basic/go

go 1.24.7

require (
	github.com/boru-lang/boru/check/go v0.0.0
	github.com/boru-lang/boru/core/go v0.0.0
	github.com/boru-lang/boru/parser/go v0.0.0
	github.com/cockroachdb/apd/v3 v3.2.3
	github.com/tabnas/parser/go v0.4.0
)

require (
	github.com/tabnas/json/go v0.4.0 // indirect
	github.com/tabnas/jsonic/go v0.4.0 // indirect
)

replace github.com/boru-lang/boru/core/go => ../../core/go

replace github.com/boru-lang/boru/check/go => ../../check/go

replace github.com/boru-lang/boru/parser/go => ../../parser/go
