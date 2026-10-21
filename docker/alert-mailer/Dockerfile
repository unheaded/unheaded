# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# alert-mailer (ADR-100). Build context is the repo root (it imports
# unheaded/pkg/...): monitoring/docker-compose.yml does this.

FROM golang:1.27.2-alpine3.24 AS builder

WORKDIR /build
COPY go.mod go.sum ./
# doomgeneric is replaced by a path outside the repo and not imported here.
RUN sed -i '/doomgeneric/d' go.mod go.sum && go mod download
COPY . .
RUN sed -i '/doomgeneric/d' go.mod go.sum && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/alert-mailer ./cmd/alert-mailer

FROM alpine:3.24

# ca-certificates: STARTTLS to the relay verifies its certificate. The
# timezone database is compiled into the binary (time/tzdata). Unpinned on
# purpose: a pinned root store keeps distrusted CAs (emSign, 2026-10-19).
# hadolint ignore=DL3018
RUN apk add --no-cache ca-certificates

# Own account (pkg/uids AlertMailer): no shell, no home.
RUN addgroup -S -g 16710 alertmailer && \
    adduser -S -D -H -h /nonexistent -s /sbin/nologin -u 16710 -G alertmailer alertmailer

COPY --from=builder /out/alert-mailer /app/alert-mailer

USER 16710:16710

EXPOSE 19130

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["wget", "--no-verbose", "--tries=1", "--spider", "http://127.0.0.1:19130/health"]

ENTRYPOINT ["/app/alert-mailer"]
