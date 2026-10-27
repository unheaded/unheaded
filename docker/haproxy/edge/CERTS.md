<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

> Lives beside `certs/`, not in it: haproxy loads every non-hidden file in a `crt` directory as a certificate, and a README there stopped the edge from starting (found live 2026-10-27).

# haproxy edge certificates

`docker/docker-compose.loadbalancers.yml` mounts this directory read-only at
`/etc/haproxy/certs`, which the edge's `bind *:21443 ssl crt /etc/haproxy/certs/`
loads (every `.pem` here: certificate chain then private key, PEM). For a dev
stack, `scripts/gen-haproxy-dev-cert.sh` writes a one-day self-signed P-256
`dev.pem` here (mode 0600; the edge runs as uid 16747, so
`sudo chown 16747:16747 docker/haproxy/edge/certs/dev.pem` before `up`).
Production certificates come from the gateway's ACME path. Never commit a key:
`.gitignore` here ignores everything except this README, and gitleaks gates the
tree. `scripts/check-haproxy-cfg.sh` never reads this directory; it generates
its own throwaway cert.
