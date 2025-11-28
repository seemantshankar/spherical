module github.com/spherical-ai/spherical/libs/knowledge-engine/tests/e2e

go 1.24.0

require (
	github.com/google/uuid v1.6.0
	github.com/mattn/go-sqlite3 v1.14.32
	github.com/spherical-ai/spherical/libs/knowledge-engine v0.0.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/redis/go-redis/v9 v9.7.0 // indirect
	github.com/rs/zerolog v1.33.0 // indirect
	golang.org/x/sys v0.36.0 // indirect
)

replace github.com/spherical-ai/spherical/libs/knowledge-engine => ../..
