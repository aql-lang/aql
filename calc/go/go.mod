module github.com/boru-lang/boru/calc/go

go 1.24.7

require github.com/boru-lang/boru/eng/go v0.0.0

require (
	github.com/cockroachdb/apd/v3 v3.2.3 // indirect
	github.com/tabnas/json/go v0.2.0 // indirect
	github.com/tabnas/jsonic/go v0.2.0 // indirect
	github.com/tabnas/parser/go v0.2.1-0.20260702232222-51f10e97d367 // indirect
)

replace github.com/boru-lang/boru/eng/go => ../../eng/go
