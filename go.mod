module unheaded

go 1.26.0

// Toolchain pin: the stdlib vulnerability surface is set here, not by
// setup-go's version selector or runner-image drift (CI reads this file with
// go-version-file). History: 1.25.10 (2026-05-08 govulncheck closure), 1.25.12
// (2026-07-29 sweep), 1.25.13 (2026-09-25).
//
// 1.27.1 on 2026-09-30: golang.org/x/crypto v0.56.0 (GO-2026-6354/-6355)
// requires go >= 1.26. The 2026-07-29 sweep deferred 1.26 because govulncheck
// "cannot type-check a 1.26 module"; that holds only for a govulncheck binary
// built by an older toolchain. Built by this one (CI's `go install` after
// setup-go does exactly that) it type-checks the tree. See
// docs/security/findings-remediation-2026-07-29.md.
//
// 1.27.2 on 2026-10-13: 13 reachable stdlib vulnerabilities in 1.27.1
// (net/http, http2, crypto/tls, mime/multipart, os, html/template;
// GO-2026-6600..6617), with golang.org/x/net v0.60.0 for the http2 ones.
toolchain go1.27.2

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/cilium/ebpf v0.22.0
	github.com/cloudflare/circl v1.6.5
	github.com/fsnotify/fsnotify v1.10.1
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/gorilla/websocket v1.5.3
	github.com/lib/pq v1.12.3
	github.com/rs/zerolog v1.35.1
	github.com/sony/gobreaker v1.0.0
	github.com/unheaded/doomgeneric v0.0.0
	github.com/yuin/goldmark v1.8.6
	golang.org/x/crypto v0.58.0
	golang.org/x/sys v0.49.0
	golang.org/x/text v0.43.0
	golang.org/x/time v0.16.0
	google.golang.org/grpc v1.84.0
	google.golang.org/protobuf v1.36.12
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.60.1
)

replace (
	github.com/unheaded/doomgeneric => ../projects/doomgeneric/unheaded
	unheaded/pkg/telemetry => ./pkg/telemetry
	unheaded/pkg/wotan-client => ./pkg/wotan-client
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/net v0.60.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260706201446-f0a921348800 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	modernc.org/libc v1.77.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)
