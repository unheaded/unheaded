# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# docker/services/doom/Dockerfile

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
    -o /out/doom ./cmd/doom

FROM debian:bookworm-slim AS certs
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.title="unheaded-doom" \
      org.opencontainers.image.description="Doom TCP service" \
      org.opencontainers.image.licenses="MIT" \
      com.unheaded.memory="1Gi"
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/doom /usr/local/bin/doom
USER nonroot:nonroot
EXPOSE 16680
ENTRYPOINT ["/usr/local/bin/doom"]
