# Busboy Message Bus Server Dockerfile
# Multi-stage build for minimal image size

# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build with optimizations
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o busboy \
    ./busboy/cmd/busboy

# Runtime stage
FROM alpine:3.19

# Security: run as non-root
RUN adduser -D -u 1000 busboy

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Copy binary
COPY --from=builder /build/busboy /usr/local/bin/busboy

# Create data directory
RUN mkdir -p /var/lib/busboy && chown busboy:busboy /var/lib/busboy

USER busboy

# Expose ports
# 8080 - REST Control Plane
# 9090 - gRPC Data Plane
EXPOSE 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/api/v1/health || exit 1

# Default command
ENTRYPOINT ["busboy"]
CMD ["--http-addr=:8080", "--grpc-addr=:9090"]
