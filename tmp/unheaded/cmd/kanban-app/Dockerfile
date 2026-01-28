# Kanban Dashboard Dockerfile
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
    -o kanban-app \
    ./cmd/kanban-app

# Runtime stage
FROM alpine:3.19

# Security: run as non-root
RUN adduser -D -u 1000 kanban

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Copy binary
COPY --from=builder /build/kanban-app /usr/local/bin/kanban-app

# Copy static files (if not embedded)
COPY --from=builder /build/kanban /app/kanban

USER kanban
WORKDIR /app

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/api/v1/health || exit 1

# Default command
ENTRYPOINT ["kanban-app"]
CMD ["--port=8080"]
