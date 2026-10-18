# tools/gosec

The gosec that CI runs (Security Scan / Go Security Scan), built from this
module instead of `go install github.com/securego/gosec/v2/cmd/gosec@vX`.

gosec v2.29.0 (the newest release) ships `golang.org/x/tools` v0.49.0, whose
importer reads Go export data up to version 4. Go 1.27 writes version 5, so
that build type-checks nothing ("export data version 5 is greater than maximum
supported version 4"), reports 0 issues and exits 1. This module requires
x/tools v0.52.0; `go.sum` pins both.

```bash
go -C tools/gosec build -o "$BIN" github.com/securego/gosec/v2/cmd/gosec
"$BIN" ./...            # from the repo root
```

Bump gosec deliberately (a release adds rules): `go -C tools/gosec get -tool
github.com/securego/gosec/v2/cmd/gosec@vX`. Keep x/tools at least at the
version that reads the current toolchain's export data.
