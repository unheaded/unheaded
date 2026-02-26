# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
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
EXPOSE 3001
CMD ["nginx", "-g", "daemon off;"]
