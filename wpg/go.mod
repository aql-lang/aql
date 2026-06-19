module github.com/aql-lang/aql/wpg

go 1.24.7

require github.com/aql-lang/aql/lang/go v0.0.0

require (
	github.com/aql-lang/aql/eng/go v0.0.0 // indirect
	github.com/cockroachdb/apd/v3 v3.2.3 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/tabnas/csv/go v0.2.0 // indirect
	github.com/tabnas/directive/go v0.2.0 // indirect
	github.com/tabnas/expr/go v0.2.0 // indirect
	github.com/tabnas/feed/go v0.2.0 // indirect
	github.com/tabnas/hoover/go v0.2.0 // indirect
	github.com/tabnas/ini/go v0.2.0 // indirect
	github.com/tabnas/json/go v0.2.0 // indirect
	github.com/tabnas/json5/go v0.2.0 // indirect
	github.com/tabnas/jsonc/go v0.2.0 // indirect
	github.com/tabnas/jsonic/go v0.2.0 // indirect
	github.com/tabnas/markdown/go v0.2.0 // indirect
	github.com/tabnas/multisource/go v0.2.0 // indirect
	github.com/tabnas/parser/go v0.2.0 // indirect
	github.com/tabnas/toml/go v0.2.0 // indirect
	github.com/tabnas/xml/go v0.2.0 // indirect
	github.com/tabnas/yaml/go v0.2.0 // indirect
	github.com/tabnas/zon/go v0.2.0 // indirect
	github.com/voxgig/struct v0.1.0 // indirect
	golang.org/x/exp v0.0.0-20251023183803-a4bb9ffd2546 // indirect
	golang.org/x/sys v0.37.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	modernc.org/libc v1.67.6 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
	modernc.org/sqlite v1.46.1 // indirect
	voxgiguniversalsdk v0.1.1 // indirect
)

replace github.com/aql-lang/aql/eng/go => ../eng/go

replace github.com/aql-lang/aql/lang/go => ../lang/go

replace github.com/voxgig/struct v0.1.0 => github.com/voxgig/struct/go v0.1.0

replace voxgiguniversalsdk v0.1.1 => github.com/voxgig/udk/go v0.1.1
