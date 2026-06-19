module github.com/aql-lang/aql/wpg

go 1.24.7

require github.com/aql-lang/aql/lang/go v0.0.0

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/aql-lang/aql/eng/go v0.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cockroachdb/apd/v3 v3.2.3 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-ini/ini v1.67.0 // indirect
	github.com/go-sql-driver/mysql v1.8.1 // indirect
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/golang-sql/civil v0.0.0-20220223132316-b832511892a9 // indirect
	github.com/golang-sql/sqlexp v0.1.0 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/itchyny/gojq v0.12.19 // indirect
	github.com/itchyny/timefmt-go v0.1.8 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.7.5 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/klauspost/cpuid/v2 v2.2.8 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/microsoft/go-mssqldb v1.8.0 // indirect
	github.com/minio/md5-simd v1.1.2 // indirect
	github.com/minio/minio-go/v7 v7.0.80 // indirect
	github.com/montanaflynn/stats v0.7.1 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/ohler55/ojg v1.28.1 // indirect
	github.com/redis/go-redis/v9 v9.7.3 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rs/xid v1.6.0 // indirect
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
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	go.etcd.io/bbolt v1.3.11 // indirect
	go.mongodb.org/mongo-driver v1.17.3 // indirect
	golang.org/x/crypto v0.37.0 // indirect
	golang.org/x/exp v0.0.0-20251023183803-a4bb9ffd2546 // indirect
	golang.org/x/net v0.30.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.24.0 // indirect
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
