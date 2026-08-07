module github.com/boru-lang/boru/basic/go

go 1.24.7

require github.com/boru-lang/boru/eng/go v0.0.0

require (
	github.com/boru-lang/boru/parser/go v0.0.0
	github.com/cockroachdb/apd/v3 v3.2.3 // indirect
	github.com/tabnas/parser/go v0.4.0 // indirect
)

replace github.com/boru-lang/boru/eng/go => ../../eng/go

replace github.com/boru-lang/boru/core/go => ../../core/go

replace github.com/boru-lang/boru/check/go => ../../check/go

replace github.com/boru-lang/boru/compiler/go => ../../compiler/go

replace github.com/boru-lang/boru/parser/go => ../../parser/go
