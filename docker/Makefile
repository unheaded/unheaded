# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# Unheaded Kingdom — Docker Makefile

VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
REGISTRY  ?= ghcr.io/stevenrbellis/unheaded

# Every service with a Dockerfile under services/ (the rest build from the
# root Dockerfile's stages)
SERVICES := monad sophia shield unheaded-daemon \
            dashboard-backend protocol-api trace-collector

BUILD_ARGS := \
  --build-arg VERSION=$(VERSION) \
  --build-arg COMMIT=$(COMMIT) \
  --build-arg BUILD_DATE=$(BUILD_DATE)

.PHONY: all build build-svc push pull logs clean preflight status help

## Default target — show help
all: help

## Build all service images
build:
	@echo "→ Building $(words $(SERVICES)) service images..."
	@echo "  Version: $(VERSION)"
	@echo "  Commit: $(COMMIT)"
	@for svc in $(SERVICES); do \
	  echo "  ▶ Building $$svc..."; \
	  docker build $(BUILD_ARGS) \
	    --build-arg SERVICE=$$svc \
	    -t unheaded-$$svc:$(VERSION) \
	    -t unheaded-$$svc:latest \
	    -f services/$$svc/Dockerfile \
	    ../ ; \
	  if [ $$? -ne 0 ]; then echo "  ✗ Failed to build $$svc"; exit 1; fi; \
	done
	@echo "✓ All $(words $(SERVICES)) images built successfully"

## Build single service: make build-svc SERVICE=monad
build-svc:
	@if [ -z "$(SERVICE)" ]; then \
	  echo "✗ SERVICE not specified. Usage: make build-svc SERVICE=<name>"; \
	  exit 1; \
	fi
	@echo "→ Building service: $(SERVICE) ($(VERSION))"
	docker build $(BUILD_ARGS) \
	  --build-arg SERVICE=$(SERVICE) \
	  -t unheaded-$(SERVICE):$(VERSION) \
	  -t unheaded-$(SERVICE):latest \
	  -f services/$(SERVICE)/Dockerfile \
	  ../
	@echo "✓ Service $(SERVICE) built successfully"

## Push all images to registry
push:
	@echo "→ Pushing $(words $(SERVICES)) service images to $(REGISTRY)..."
	@for svc in $(SERVICES); do \
	  echo "  ▶ Pushing $$svc:$(VERSION)"; \
	  docker tag unheaded-$$svc:$(VERSION) $(REGISTRY)/$$svc:$(VERSION); \
	  docker push $(REGISTRY)/$$svc:$(VERSION); \
	  if [ $$? -ne 0 ]; then echo "  ✗ Failed to push $$svc"; exit 1; fi; \
	done
	@echo "✓ All images pushed to $(REGISTRY)"

## Pull all images from registry
pull:
	@echo "→ Pulling $(words $(SERVICES)) service images from $(REGISTRY)..."
	@for svc in $(SERVICES); do \
	  echo "  ▶ Pulling $$svc:$(VERSION)"; \
	  docker pull $(REGISTRY)/$$svc:$(VERSION); \
	  docker tag $(REGISTRY)/$$svc:$(VERSION) unheaded-$$svc:$(VERSION); \
	  docker tag unheaded-$$svc:$(VERSION) unheaded-$$svc:latest; \
	done
	@echo "✓ All images pulled"

## Tail logs for a service: make logs SERVICE=monad
logs:
	@if [ -z "$(SERVICE)" ]; then \
	  echo "✗ SERVICE not specified. Usage: make logs SERVICE=<name>"; \
	  exit 1; \
	fi
	docker logs -f unheaded-$(SERVICE)

## Run pre-flight checks for deployment
preflight:
	@echo "→ Running pre-flight checks..."
	@if ! command -v docker &> /dev/null; then \
	  echo "✗ docker not found"; \
	  exit 1; \
	fi
	@if ! command -v docker compose &> /dev/null; then \
	  echo "✗ docker compose not found"; \
	  exit 1; \
	fi
	@DOCKER_VERSION=$$(docker --version | grep -oP '\d+\.\d+' | head -1); \
	echo "  Docker version: $$DOCKER_VERSION"; \
	COMPOSE_VERSION=$$(docker compose version | grep -oP '\d+\.\d+' | head -1); \
	echo "  Docker Compose version: $$COMPOSE_VERSION"; \
	KERNEL_VERSION=$$(uname -r | grep -oP '\d+\.\d+' | head -1); \
	echo "  Kernel version: $$KERNEL_VERSION"; \
	@echo "  Cgroups v2: $$(grep -c cgroup2 /proc/mounts || echo 'not detected')"; \
	@echo "  IPv6 support: $$(ip -6 addr show | wc -l > 0 && echo 'enabled' || echo 'disabled')"; \
	@echo "  /dev/kfd (ROCm): $$(test -e /dev/kfd && echo 'available' || echo 'not available')"; \
	@echo "✓ Pre-flight checks complete"

## Show status of all containers
status:
	@echo "→ Unheaded service status:"
	@docker ps --filter "name=unheaded-" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" || echo "No unheaded containers running"

## Clean all unheaded images (be careful!)
clean:
	@echo "⚠ WARNING: This will remove all unheaded Docker images"
	@read -p "Continue? [y/N] " -n 1 -r; \
	echo; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
	  for svc in $(SERVICES); do \
	    docker rmi unheaded-$$svc:$(VERSION) unheaded-$$svc:latest 2>/dev/null || true; \
	  done; \
	  echo "✓ Cleanup complete"; \
	else \
	  echo "✗ Cleanup cancelled"; \
	fi

## Show help
help:
	@echo ""
	@echo "Unheaded Kingdom — Docker Management"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' Makefile | sed 's/## /  /'
	@echo ""
	@echo "Examples:"
	@echo "  make build                    # Build all 25 services"
	@echo "  make build-svc SERVICE=monad  # Build single service"
	@echo "  make logs SERVICE=monad       # Follow logs for service"
	@echo "  make status                   # Show running containers"
	@echo ""
