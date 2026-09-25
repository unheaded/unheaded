module unheaded

go 1.25.0

// Toolchain pin. Originally 1.25.10 for the 2026-05-08 govulncheck closure
// (~33 of 35 stdlib advisories), then 1.25.12 during the 2026-07-29 security
// sweep to pin GO-2026-5856 / GO-2026-5039 / GO-2026-5037.
//
// Still 1.25.12. Go 1.26.5 was EVALUATED during the 2026-07-29 sweep and
// DEFERRED: govulncheck cannot type-check a 1.26 module, so adopting it would
// have blinded the very gate the pin exists to serve. See
// docs/security/findings-remediation-2026-07-29.md for that decision.
//
// (This comment previously read "Now 1.26.5, the current stable line", which
// contradicted the toolchain directive one line below it and would have told
// anyone auditing the stdlib CVE surface that the wrong toolchain was in force.)
//
// Pinning here rather than relying on setup-go's version selector means the
// stdlib vulnerability surface is deterministic instead of dependent on
// runner-image drift. See docs/security/govulncheck-2026-05-08.md for the
// original analysis.
toolchain go1.25.12

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/cilium/ebpf v0.20.0
	github.com/cloudflare/circl v1.6.3
	github.com/fsnotify/fsnotify v1.7.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/gorilla/websocket v1.5.3
	github.com/lib/pq v1.10.9
	github.com/rs/zerolog v1.31.0
	github.com/sony/gobreaker v0.5.0
	github.com/unheaded/doomgeneric v0.0.0
	github.com/yuin/goldmark v1.7.17
	golang.org/x/crypto v0.55.0
	golang.org/x/sys v0.47.0
	golang.org/x/text v0.41.0
	golang.org/x/time v0.5.0
	google.golang.org/grpc v1.83.2
	google.golang.org/protobuf v1.36.11
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.44.3
)

replace (
	github.com/unheaded/doomgeneric => ../projects/doomgeneric/unheaded
	unheaded/pkg/telemetry => ./pkg/telemetry
	unheaded/pkg/wotan-client => ./pkg/wotan-client
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/exp v0.0.0-20251023183803-a4bb9ffd2546 // indirect
	golang.org/x/net v0.58.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	modernc.org/libc v1.67.6 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
