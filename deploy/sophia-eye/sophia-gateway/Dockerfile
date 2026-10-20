# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# sophia-gateway imports unheaded/pkg/..., so the build context is the repo
# root: docker build -f deploy/sophia-eye/sophia-gateway/Dockerfile .
# (deploy/sophia-eye/docker-compose.yml does this). Its old context,
# ./sophia-gateway, had no Dockerfile and no go.mod.

FROM golang:1.27-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
# doomgeneric is replaced by a path outside the repo; the gateway does not
# import it, so drop it rather than resolve it (as the root Dockerfile does).
RUN sed -i '/doomgeneric/d' go.mod go.sum && go mod download
COPY . .
RUN sed -i '/doomgeneric/d' go.mod go.sum && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' \
      -o /out/sophia-gateway ./deploy/sophia-eye/sophia-gateway

FROM alpine:3.24

RUN apk add --no-cache ca-certificates tzdata

# Own account (pkg/uids SophiaGateway): no shell, no home.
RUN addgroup -S -g 16733 sophiagw && \
    adduser -S -D -H -h /nonexistent -s /sbin/nologin -u 16733 -G sophiagw sophiagw

COPY --from=builder /out/sophia-gateway /app/sophia-gateway

USER 16733:16733

EXPOSE 20105 9090

HEALTHCHECK --interval=10s --timeout=5s --start-period=20s --retries=5 \
    CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:9090/health || exit 1

ENTRYPOINT ["/app/sophia-gateway"]
