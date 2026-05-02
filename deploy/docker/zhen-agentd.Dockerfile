# SPDX-License-Identifier: GPL-3.0-or-later
# Multi-stage build for zhen-agentd. Final image is distroless-static
# (~18 MB) and runs as a non-root user.
#
# Build:
#   docker build -f deploy/docker/zhen-agentd.Dockerfile -t zhen-agentd:dev .
#
# Run:
#   docker run --rm -p 20105:20105 \
#     -e AUTH_ENABLED=false \
#     -e VOR_URL=http://host.docker.internal:9876 \
#     -e LLAMA_URL=http://host.docker.internal:8081 \
#     zhen-agentd:dev
#
# Production deploy: behind an nginx/HAProxy sidecar for TLS termination
# (per Unheaded's Port-Authority pattern).

FROM golang:1.24-alpine AS builder

# Build deps (CGO disabled — we want a pure-static binary).
RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build with the same flags as the Makefile target.
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build \
    -trimpath \
    -ldflags "-s -w" \
    -o /out/zhen-agentd \
    ./cmd/zhen-agentd/

# --- Runtime stage ---
FROM gcr.io/distroless/static-debian12:nonroot

# Distroless ships ca-certificates already, but copy them explicitly
# so the image is reproducible across base updates.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=builder /out/zhen-agentd /usr/local/bin/zhen-agentd

USER nonroot:nonroot
EXPOSE 20105

# Default command. Override at run time via docker -e flags or
# kubernetes env. Keep -host 0.0.0.0 so the container's port is
# reachable from the host network.
ENTRYPOINT ["/usr/local/bin/zhen-agentd"]
CMD ["-host", "0.0.0.0", "-port", "20105", \
     "-project-root", "/var/zhen/projects/default", \
     "-action-store=stderr", \
     "-rate-limit", "10", \
     "-rate-burst", "25"]
