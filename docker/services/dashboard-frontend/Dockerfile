# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# docker/services/dashboard-frontend/Dockerfile

FROM node:22-alpine AS builder
WORKDIR /build
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:alpine
LABEL org.opencontainers.image.title="unheaded-dashboard-frontend" \
      org.opencontainers.image.description="Dashboard frontend web UI" \
      org.opencontainers.image.licenses="MIT"
COPY --from=builder /build/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Run as the image's unprivileged nginx user (trivy DS-0002).
#
# This container serves static files on 3001 — a port above 1024, so nothing
# here ever needed root. The stock nginx image runs its master process as root
# purely so it can bind :80 and write /var/run/nginx.pid; neither applies.
#
# Two paths must be relocated or made writable for a non-root master:
# the PID file (default /var/run/nginx.pid, root-owned) and the proxy cache
# dirs. Both move under /tmp/nginx, which nginx owns.
RUN mkdir -p /tmp/nginx /var/cache/nginx \
    && chown -R nginx:nginx /tmp/nginx /var/cache/nginx /usr/share/nginx/html \
    && sed -i 's,^pid .*,pid /tmp/nginx/nginx.pid;,' /etc/nginx/nginx.conf

USER nginx

EXPOSE 3001
CMD ["nginx", "-g", "daemon off;"]
