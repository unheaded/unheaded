# ============================================================================
# THE UNHEADED KINGDOM - Multi-stage Dockerfile
# "Configuration management automation platform"
# ============================================================================

# ============================================================================
# STAGE 1: BUILD - THE FORGE
# ============================================================================
FROM golang:1.27.3-alpine3.24 AS builder

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
# Remove local replace directives that reference paths outside the build context
COPY go.mod go.sum ./
RUN sed -i '/=> \.\.\//d' go.mod && go mod download || true

# Copy source code
COPY . .
# Remove local replace directives and doomgeneric references for container builds
RUN sed -i '/=> \.\.\//d' go.mod && \
    sed -i '/doomgeneric/d' go.mod && \
    go mod tidy -e 2>/dev/null || true

# Build arguments for version injection
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

# Build all services: one RUN, one layer (every build depends on the COPY above).
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/wotan ./services/wotan/cmd/wotan && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/timeguru ./services/timeguru/cmd/timeguru && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/captain ./services/captain/cmd/captain && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/architect ./services/architect/cmd/architect && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/micromanager ./services/micromanager/cmd/micromanager && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/unheaded-daemon ./cmd/unheaded-daemon && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/monad ./cmd/monad && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/sophia ./cmd/sophia && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.Version=${VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o /build/bin/dashboard-backend ./cmd/dashboard-backend && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/kanban-app ./cmd/kanban-app && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/akira ./cmd/akira && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/kev-poller ./cmd/kev-poller && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/sword ./cmd/sword && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /build/bin/unheaded-auth ./cmd/unheaded-auth && \
    CGO_ENABLED=0 GOOS=linux go build \
    -o /build/bin/healthprobe ./cmd/healthprobe

# ============================================================================
# RUNTIME BASE - what every service image starts from
# ============================================================================
# Every service stage installed the same two packages; this stage does it
# once. Versions float within the pinned alpine:3.24 branch, so security
# fixes land on rebuild.
FROM alpine:3.24 AS runtime

RUN apk add --no-cache ca-certificates tzdata

# The healthcheck every service image uses (S-IRON-GATE Phase 8): busybox
# wget cannot verify a TLS listener against the deployment CA.
COPY --from=builder /build/bin/healthprobe /usr/local/bin/healthprobe

# ============================================================================
# STAGE 2: WOTAN - THE FAE CHAMBER
# ============================================================================
FROM runtime AS wotan

# Own account (pkg/uids): no shell, no home, uid shared with nothing else.
RUN addgroup -S -g 16720 wotan && \
    adduser -S -D -H -h /nonexistent -s /sbin/nologin -u 16720 -G wotan wotan

WORKDIR /app

COPY --from=builder /build/bin/wotan /app/wotan

# Create data directory
RUN mkdir -p /data && chown 16720:16720 /data

USER 16720:16720

EXPOSE 18000 18001

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/healthprobe", "18000"]

ENTRYPOINT ["/app/wotan"]

# ============================================================================
# STAGE 3: TIMEGURU - THE ORACLE'S ANTRE
# ============================================================================
FROM runtime AS timeguru

# Own account (pkg/uids): no shell, no home, uid shared with nothing else.
RUN addgroup -S -g 16722 timeguru && \
    adduser -S -D -H -h /nonexistent -s /sbin/nologin -u 16722 -G timeguru timeguru

WORKDIR /app

COPY --from=builder /build/bin/timeguru /app/timeguru

RUN mkdir -p /data && chown 16722:16722 /data

USER 16722:16722

EXPOSE 19000

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/healthprobe", "19000"]

ENTRYPOINT ["/app/timeguru"]

# ============================================================================
# STAGE 4: CAPTAIN - THE COMMANDER'S QUARTERS
# ============================================================================
FROM runtime AS captain

# Own account (pkg/uids): no shell, no home, uid shared with nothing else.
RUN addgroup -S -g 16724 captain && \
    adduser -S -D -H -h /nonexistent -s /sbin/nologin -u 16724 -G captain captain

WORKDIR /app

COPY --from=builder /build/bin/captain /app/captain

RUN mkdir -p /data && chown 16724:16724 /data

USER 16724:16724

EXPOSE 19002

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/healthprobe", "19002"]

ENTRYPOINT ["/app/captain"]

# ============================================================================
# STAGE 5: ARCHITECT - THE SAGE'S LAIR
# ============================================================================
FROM runtime AS architect

# Own account (pkg/uids): no shell, no home, uid shared with nothing else.
RUN addgroup -S -g 16723 architect && \
    adduser -S -D -H -h /nonexistent -s /sbin/nologin -u 16723 -G architect architect

WORKDIR /app

COPY --from=builder /build/bin/architect /app/architect

RUN mkdir -p /data && chown 16723:16723 /data

USER 16723:16723

EXPOSE 19001

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/healthprobe", "19001"]

ENTRYPOINT ["/app/architect"]

# ============================================================================
# STAGE 6: MICROMANAGER - THE WAR ROOM
# ============================================================================
# Akira: consensus health (ADR-029). Checks the services and tallies every
# reporter's votes; remediation is off unless started with --remediate.
FROM runtime AS akira

# Own account (pkg/uids): no shell, no home, uid shared with nothing else.
RUN addgroup -S -g 16731 akira && \
    adduser -S -D -H -h /nonexistent -s /sbin/nologin -u 16731 -G akira akira

WORKDIR /app

COPY --from=builder /build/bin/akira /app/akira

USER 16731:16731

EXPOSE 19100

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/healthprobe", "19100"]

ENTRYPOINT ["/app/akira"]

# ============================================================================
# KEV-POLLER: CISA Known Exploited Vulnerabilities feed (ADR-055). Writes to
# /data (mount a volume there); outbound https to www.cisa.gov only.
FROM runtime AS kev-poller

# Own account (pkg/uids): no shell, no home, uid shared with nothing else.
RUN addgroup -S -g 16732 kevpoller && \
    adduser -S -D -H -h /nonexistent -s /sbin/nologin -u 16732 -G kevpoller kevpoller

WORKDIR /app

COPY --from=builder /build/bin/kev-poller /app/kev-poller
COPY config/kev-poller/stack.yaml /etc/unheaded/kev-stack.yaml

RUN mkdir -p /data && chown 16732:16732 /data && chmod 0750 /data

USER 16732:16732

EXPOSE 19120

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/healthprobe", "http://127.0.0.1:19120/health"]

ENTRYPOINT ["/app/kev-poller", "--state-dir", "/data", "--stack", "/etc/unheaded/kev-stack.yaml"]

# ============================================================================
FROM runtime AS micromanager

# Own account (pkg/uids): no shell, no home, uid shared with nothing else.
RUN addgroup -S -g 16725 micromanager && \
    adduser -S -D -H -h /nonexistent -s /sbin/nologin -u 16725 -G micromanager micromanager

WORKDIR /app

COPY --from=builder /build/bin/micromanager /app/micromanager

RUN mkdir -p /data && chown 16725:16725 /data

USER 16725:16725

EXPOSE 19003

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/healthprobe", "19003"]

ENTRYPOINT ["/app/micromanager"]

# ============================================================================
# STAGE 7: MONAD - THE ONE (Unified State Management)
# ============================================================================
FROM runtime AS monad

# Own account (pkg/uids): no shell, no home, uid shared with nothing else.
RUN addgroup -S -g 16726 monad && \
    adduser -S -D -H -h /nonexistent -s /sbin/nologin -u 16726 -G monad monad

WORKDIR /app

COPY --from=builder /build/bin/monad /app/monad

RUN mkdir -p /data && chown 16726:16726 /data

USER 16726:16726

EXPOSE 19004

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/healthprobe", "19004"]

ENTRYPOINT ["/app/monad"]

# ============================================================================
# STAGE 8: SOPHIA - DIVINE WISDOM (Knowledge Graph)
# ============================================================================
FROM runtime AS sophia

# Own account (pkg/uids): no shell, no home, uid shared with nothing else.
RUN addgroup -S -g 16727 sophia && \
    adduser -S -D -H -h /nonexistent -s /sbin/nologin -u 16727 -G sophia sophia

WORKDIR /app

COPY --from=builder /build/bin/sophia /app/sophia

RUN mkdir -p /data && chown 16727:16727 /data

USER 16727:16727

EXPOSE 19005

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/healthprobe", "19005"]

ENTRYPOINT ["/app/sophia"]

# ============================================================================
# STAGE 9: CUIRASS - THE CORE HEART (Control Plane)
# ============================================================================
FROM runtime AS cuirass

# Own account (pkg/uids): no shell, no home, uid shared with nothing else.
RUN addgroup -S -g 16740 cuirass && \
    adduser -S -D -H -h /nonexistent -s /sbin/nologin -u 16740 -G cuirass cuirass

WORKDIR /app

COPY --from=builder /build/bin/unheaded-daemon /app/unheaded-daemon

RUN mkdir -p /data /var/lib/unheaded && \
    chown -R 16740:16740 /data /var/lib/unheaded

USER 16740:16740

EXPOSE 17000 17001

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/healthprobe", "17000"]

ENTRYPOINT ["/app/unheaded-daemon"]

# ============================================================================
# SWORD - the deploy service (S-REAL-STEEL Phase 8, D-09)
# ============================================================================
FROM runtime AS sword

# Own account (pkg/uids.Sword): no shell, no home, uid shared with nothing else.
RUN addgroup -S -g 16746 sword && \
    adduser -S -D -H -h /nonexistent -s /sbin/nologin -u 16746 -G sword sword

WORKDIR /app

COPY --from=builder /build/bin/sword /app/sword

# The deployment history (SWORD_DATA_DIR); a named volume inherits this owner.
RUN mkdir -p /data && chown 16746:16746 /data && chmod 0750 /data

USER 16746:16746

EXPOSE 19011

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/healthprobe", "19011"]

ENTRYPOINT ["/app/sword"]

# ============================================================================
# STAGE 10: DASHBOARD-BACKEND - THE VISOR (Observability Dashboard)
# ============================================================================
FROM runtime AS dashboard-backend

# Own account (pkg/uids): no shell, no home, uid shared with nothing else.
RUN addgroup -S -g 16728 dashboard-backend && \
    adduser -S -D -H -h /nonexistent -s /sbin/nologin -u 16728 -G dashboard-backend dashboard-backend

WORKDIR /app

COPY --from=builder /build/bin/dashboard-backend /app/dashboard-backend
# Copy advanced visualization files for --viz-dir
COPY --from=builder /build/dashboard/ /app/viz/

RUN mkdir -p /data && chown 16728:16728 /data

USER 16728:16728

EXPOSE 20000

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/healthprobe", "20000"]

ENTRYPOINT ["/app/dashboard-backend", "--viz-dir", "/app/viz"]

# ============================================================================
# STAGE 11: KANBAN-APP - THE META MOMENT (Self-Hosting Proof)
# ============================================================================
FROM runtime AS kanban-app

# Own account (pkg/uids): no shell, no home, uid shared with nothing else.
RUN addgroup -S -g 16729 kanban-app && \
    adduser -S -D -H -h /nonexistent -s /sbin/nologin -u 16729 -G kanban-app kanban-app

WORKDIR /app

COPY --from=builder /build/bin/kanban-app /app/kanban-app
# The Runbooks view lists these (KANBAN_RUNBOOKS_DIR defaults to ./runbooks);
# read-only, root-owned, the manifest says which bytes are the committed ones.
COPY runbooks /app/runbooks

RUN mkdir -p /data && chown 16729:16729 /data

USER 16729:16729

EXPOSE 20001

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/healthprobe", "20001"]

ENTRYPOINT ["/app/kanban-app"]

# ============================================================================
# UNHEADED-AUTH - the built-in OIDC issuer (ADR-103)
# ============================================================================
FROM runtime AS unheaded-auth

# Own account (pkg/uids): no shell, no home, uid shared with nothing else.
RUN addgroup -S -g 16735 unheaded-auth && \
    adduser -S -D -H -h /nonexistent -s /sbin/nologin -u 16735 -G unheaded-auth unheaded-auth

WORKDIR /app

COPY --from=builder /build/bin/unheaded-auth /app/unheaded-auth

USER 16735:16735

EXPOSE 19020

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/healthprobe", "19020"]

ENTRYPOINT ["/app/unheaded-auth"]

# ============================================================================
# DEFAULT: ALL-IN-ONE (for development)
# ============================================================================
FROM runtime

RUN apk add --no-cache supervisor

# One container running every service under supervisord (dev only), so one
# account for all of them; still not root and not a login account (pkg/uids).
RUN addgroup -S -g 16759 unheaded-dev && \
    adduser -S -D -H -h /nonexistent -s /sbin/nologin -u 16759 -G unheaded-dev unheaded-dev

WORKDIR /app

# Copy all binaries
COPY --from=builder /build/bin/* /app/

# Create directories
RUN mkdir -p /data /var/lib/unheaded /etc/supervisor.d && \
    chown -R 16759:16759 /data /var/lib/unheaded

# Supervisor config for all-in-one (a file, not a RUN heredoc: hadolint
# cannot parse heredocs, so this Dockerfile was never linted).
COPY config/supervisor/all-in-one.ini /etc/supervisor.d/unheaded.ini

# What the programs in all-in-one.ini listen on at their defaults (pkg/ports):
# unheaded-daemon 17000, wotan 18000/18001, timeguru..sophia 19000-19005,
# dashboard 20000, kanban 20001.
EXPOSE 17000 18000 18001 19000 19001 19002 19003 19004 19005 20000 20001

USER 16759:16759

# Our file only: the stock /etc/supervisord.conf logs and pids under root-owned paths.
ENTRYPOINT ["/usr/bin/supervisord", "-c", "/etc/supervisor.d/unheaded.ini"]
