# ============================================================================
# THE UNHEADED KINGDOM - Multi-stage Dockerfile
# "Production-ready in hours, not months"
# ============================================================================

# ============================================================================
# STAGE 1: BUILD - THE FORGE
# ============================================================================
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache \
    git \
    make \
    gcc \
    musl-dev \
    linux-headers

# Set working directory
WORKDIR /build

# Copy go mod files first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build arguments for version injection
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

# Build all services
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/busboy ./services/busboy/cmd/busboy

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/timeguru ./services/timeguru/cmd/timeguru

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/captain ./services/captain/cmd/captain

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/architect ./services/architect/cmd/architect

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/micromanager ./services/micromanager/cmd/micromanager

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/unheaded-daemon ./cmd/unheaded-daemon

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/monad ./cmd/monad

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/sophia ./cmd/sophia

# ============================================================================
# STAGE 2: BUSBOY - THE FAE CHAMBER
# ============================================================================
FROM alpine:3.19 AS busboy

RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 unheaded && \
    adduser -u 1000 -G unheaded -s /bin/sh -D unheaded

WORKDIR /app

COPY --from=builder /build/bin/busboy /app/busboy

# Create data directory
RUN mkdir -p /data && chown unheaded:unheaded /data

USER unheaded

EXPOSE 8080 9090

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/busboy"]

# ============================================================================
# STAGE 3: TIMEGURU - THE ORACLE'S ANTRE
# ============================================================================
FROM alpine:3.19 AS timeguru

RUN apk add --no-cache ca-certificates tzdata

RUN addgroup -g 1000 unheaded && \
    adduser -u 1000 -G unheaded -s /bin/sh -D unheaded

WORKDIR /app

COPY --from=builder /build/bin/timeguru /app/timeguru

RUN mkdir -p /data && chown unheaded:unheaded /data

USER unheaded

EXPOSE 8082

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8082/health || exit 1

ENTRYPOINT ["/app/timeguru"]

# ============================================================================
# STAGE 4: CAPTAIN - THE COMMANDER'S QUARTERS
# ============================================================================
FROM alpine:3.19 AS captain

RUN apk add --no-cache ca-certificates tzdata

RUN addgroup -g 1000 unheaded && \
    adduser -u 1000 -G unheaded -s /bin/sh -D unheaded

WORKDIR /app

COPY --from=builder /build/bin/captain /app/captain

RUN mkdir -p /data && chown unheaded:unheaded /data

USER unheaded

EXPOSE 8083

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8083/health || exit 1

ENTRYPOINT ["/app/captain"]

# ============================================================================
# STAGE 5: ARCHITECT - THE SAGE'S LAIR
# ============================================================================
FROM alpine:3.19 AS architect

RUN apk add --no-cache ca-certificates tzdata

RUN addgroup -g 1000 unheaded && \
    adduser -u 1000 -G unheaded -s /bin/sh -D unheaded

WORKDIR /app

COPY --from=builder /build/bin/architect /app/architect

RUN mkdir -p /data && chown unheaded:unheaded /data

USER unheaded

EXPOSE 8084

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8084/health || exit 1

ENTRYPOINT ["/app/architect"]

# ============================================================================
# STAGE 6: MICROMANAGER - THE WAR ROOM
# ============================================================================
FROM alpine:3.19 AS micromanager

RUN apk add --no-cache ca-certificates tzdata

RUN addgroup -g 1000 unheaded && \
    adduser -u 1000 -G unheaded -s /bin/sh -D unheaded

WORKDIR /app

COPY --from=builder /build/bin/micromanager /app/micromanager

RUN mkdir -p /data && chown unheaded:unheaded /data

USER unheaded

EXPOSE 8085

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8085/health || exit 1

ENTRYPOINT ["/app/micromanager"]

# ============================================================================
# STAGE 7: MONAD - THE ONE (Unified State Management)
# ============================================================================
FROM alpine:3.19 AS monad

RUN apk add --no-cache ca-certificates tzdata

RUN addgroup -g 1000 unheaded && \
    adduser -u 1000 -G unheaded -s /bin/sh -D unheaded

WORKDIR /app

COPY --from=builder /build/bin/monad /app/monad

RUN mkdir -p /data && chown unheaded:unheaded /data

USER unheaded

EXPOSE 8086

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8086/health || exit 1

ENTRYPOINT ["/app/monad"]

# ============================================================================
# STAGE 8: SOPHIA - DIVINE WISDOM (Knowledge Graph)
# ============================================================================
FROM alpine:3.19 AS sophia

RUN apk add --no-cache ca-certificates tzdata

RUN addgroup -g 1000 unheaded && \
    adduser -u 1000 -G unheaded -s /bin/sh -D unheaded

WORKDIR /app

COPY --from=builder /build/bin/sophia /app/sophia

RUN mkdir -p /data && chown unheaded:unheaded /data

USER unheaded

EXPOSE 8087

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8087/health || exit 1

ENTRYPOINT ["/app/sophia"]

# ============================================================================
# STAGE 9: CUIRASS - THE CORE HEART (Control Plane)
# ============================================================================
FROM alpine:3.19 AS cuirass

RUN apk add --no-cache ca-certificates tzdata

RUN addgroup -g 1000 unheaded && \
    adduser -u 1000 -G unheaded -s /bin/sh -D unheaded

WORKDIR /app

COPY --from=builder /build/bin/unheaded-daemon /app/unheaded-daemon

RUN mkdir -p /data /var/lib/unheaded && \
    chown -R unheaded:unheaded /data /var/lib/unheaded

USER unheaded

EXPOSE 8080 9090

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/unheaded-daemon"]

# ============================================================================
# DEFAULT: ALL-IN-ONE (for development)
# ============================================================================
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata supervisor

RUN addgroup -g 1000 unheaded && \
    adduser -u 1000 -G unheaded -s /bin/sh -D unheaded

WORKDIR /app

# Copy all binaries
COPY --from=builder /build/bin/* /app/

# Create directories
RUN mkdir -p /data /var/lib/unheaded /etc/supervisor.d && \
    chown -R unheaded:unheaded /data /var/lib/unheaded

# Supervisor config for all-in-one
RUN cat > /etc/supervisor.d/unheaded.ini << 'EOF'
[supervisord]
nodaemon=true

[program:busboy]
command=/app/busboy
autostart=true
autorestart=true
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0

[program:timeguru]
command=/app/timeguru
autostart=true
autorestart=true
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0

[program:captain]
command=/app/captain
autostart=true
autorestart=true
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0

[program:architect]
command=/app/architect
autostart=true
autorestart=true
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0

[program:micromanager]
command=/app/micromanager
autostart=true
autorestart=true
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0

[program:monad]
command=/app/monad
autostart=true
autorestart=true
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0

[program:sophia]
command=/app/sophia
autostart=true
autorestart=true
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0

[program:cuirass]
command=/app/unheaded-daemon
autostart=true
autorestart=true
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
EOF

EXPOSE 5555 8080 8081 8082 8083 8084 8085 8086 8087 9090

ENTRYPOINT ["/usr/bin/supervisord", "-c", "/etc/supervisord.conf"]
