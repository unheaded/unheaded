module unheaded

go 1.25.0

// Toolchain pin. Originally 1.25.10 for the 2026-05-08 govulncheck closure
// (~33 of 35 stdlib advisories), then 1.25.12 during the 2026-07-29 security
// sweep to pin GO-2026-5856 / GO-2026-5039 / GO-2026-5037.
//
// Now 1.26.5, the current stable line. Pinning here rather than relying on
// setup-go's '1.26' means the stdlib vulnerability surface is deterministic
// instead of dependent on runner-image drift. See
// docs/security/govulncheck-2026-05-08.md for the original analysis.
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
	github.com/prometheus/client_golang v1.18.0
	github.com/rs/zerolog v1.31.0
	github.com/sony/gobreaker v0.5.0
	github.com/unheaded/doomgeneric v0.0.0
	github.com/yuin/goldmark v1.7.17
	golang.org/x/crypto v0.53.0
	golang.org/x/sys v0.46.0
	golang.org/x/text v0.39.0
	golang.org/x/time v0.5.0
	google.golang.org/grpc v1.82.1
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
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/matttproud/golang_protobuf_extensions/v2 v2.0.0 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.45.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/exp v0.0.0-20251023183803-a4bb9ffd2546 // indirect
	golang.org/x/net v0.56.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260414002931-afd174a4e478 // indirect
	modernc.org/libc v1.67.6 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
