#!/bin/bash
# Generate Go code from protobuf definitions

set -e

# Add GOPATH/bin to PATH for protoc plugins
export PATH=$PATH:$(go env GOPATH)/bin

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
       chat.proto topic.proto

echo "Done! Generated files:"
ls -lh *.pb.go
