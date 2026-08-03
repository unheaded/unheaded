#!/bin/bash
# Generate Go code from protobuf definitions

set -e

# Add GOPATH/bin to PATH for protoc plugins. Split from the export so a failing
# `go env` is visible rather than silently producing PATH=$PATH:/bin.
GOPATH_BIN="$(go env GOPATH)/bin"
export PATH="${PATH}:${GOPATH_BIN}"

# Check if protoc is installed
if ! command -v protoc &> /dev/null; then
    echo "protoc not found. Please install protobuf compiler."
    echo "  MacOS: brew install protobuf"
    echo "  Linux: apt-get install -y protobuf-compiler"
    exit 1
fi

# Check if protoc-gen-go is installed
if ! command -v protoc-gen-go &> /dev/null; then
    echo "protoc-gen-go not found. Installing..."
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
fi

# Check if protoc-gen-go-grpc is installed
if ! command -v protoc-gen-go-grpc &> /dev/null; then
    echo "protoc-gen-go-grpc not found. Installing..."
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
fi

echo "Generating Go code from protobuf..."

protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       chat.proto topic.proto replication.proto

# Prepend SPDX header to every generated .pb.go file. protoc-gen-go does not
# support a header template, so we post-process. Idempotent: skips files that
# already start with the SPDX line.
SPDX_LINE="// SPDX-License-Identifier: GPL-3.0-or-later"
for f in *.pb.go; do
    if ! head -n 1 "$f" | grep -q "SPDX-License-Identifier"; then
        tmp="$(mktemp)"
        { echo "$SPDX_LINE"; cat "$f"; } > "$tmp"
        mv "$tmp" "$f"
        echo "  + injected SPDX header into $f"
    fi
done

echo "Done! Generated files:"
ls -lh *.pb.go
