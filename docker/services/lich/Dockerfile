# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# docker/services/lich/Dockerfile
# Adversary scanner service — requires network capabilities

ARG GO_VERSION=1.24.0
ARG VERSION=dev
ARG BUILD_DATE=unknown
ARG COMMIT=unknown

FROM golang:${GO_VERSION}-bookworm AS builder
ARG VERSION BUILD_DATE COMMIT
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -buildvcs=false \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildDate=${BUILD_DATE} -X main.Commit=${COMMIT} -extldflags '-static'" \
    -o /out/lich ./cmd/lich

FROM debian:bookworm-slim AS final
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -g 10001 unheaded \
    && useradd -u 10001 -g unheaded -s /sbin/nologin -r unheaded
LABEL org.opencontainers.image.title="unheaded-lich" \
      org.opencontainers.image.description="Lich adversary scanner service" \
      org.opencontainers.image.licenses="MIT"
COPY --from=builder /out/lich /usr/local/bin/lich
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/lich"]
