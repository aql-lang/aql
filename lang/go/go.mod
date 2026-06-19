module github.com/aql-lang/aql/lang/go

go 1.24.7

require (
	github.com/aql-lang/aql/eng/go v0.0.0
	github.com/cockroachdb/apd/v3 v3.2.3
	github.com/go-sql-driver/mysql v1.8.1
	github.com/itchyny/gojq v0.12.19
	github.com/jackc/pgx/v5 v5.7.5
	github.com/microsoft/go-mssqldb v1.8.0
	github.com/ohler55/ojg v1.28.1
	github.com/redis/go-redis/v9 v9.7.3
	github.com/tabnas/csv/go v0.2.0
	github.com/tabnas/expr/go v0.2.0
	github.com/tabnas/feed/go v0.2.0
	github.com/tabnas/ini/go v0.2.0
	github.com/tabnas/json/go v0.2.0
	github.com/tabnas/json5/go v0.2.0
	github.com/tabnas/jsonc/go v0.2.0
	github.com/tabnas/jsonic/go v0.2.0
	github.com/tabnas/markdown/go v0.2.0
	github.com/tabnas/multisource/go v0.2.0
	github.com/tabnas/toml/go v0.2.0
	github.com/tabnas/xml/go v0.2.0
	github.com/tabnas/yaml/go v0.2.0
	github.com/tabnas/zon/go v0.2.0
	go.etcd.io/bbolt v1.3.11
	golang.org/x/text v0.24.0
	modernc.org/sqlite v1.46.1
	voxgiguniversalsdk v0.1.1
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/alicebob/gopher-json v0.0.0-20200520072559-a9ecdc9d1d3a // indirect
	github.com/alicebob/miniredis/v2 v2.33.0 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/golang-sql/civil v0.0.0-20220223132316-b832511892a9 // indirect
	github.com/golang-sql/sqlexp v0.1.0 // indirect
	github.com/itchyny/timefmt-go v0.1.8 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/tabnas/directive/go v0.2.0 // indirect
	github.com/tabnas/hoover/go v0.2.0 // indirect
	github.com/tabnas/parser/go v0.2.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	golang.org/x/crypto v0.37.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/voxgig/struct v0.1.0
	golang.org/x/exp v0.0.0-20251023183803-a4bb9ffd2546 // indirect
	golang.org/x/sys v0.38.0 // indirect
	modernc.org/libc v1.67.6 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace github.com/voxgig/struct v0.1.0 => github.com/voxgig/struct/go v0.1.0

replace voxgiguniversalsdk v0.1.1 => github.com/voxgig/udk/go v0.1.1

replace github.com/aql-lang/aql/eng/go => ../../eng/go
