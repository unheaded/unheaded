# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# Unheaded Kingdom — Docker Makefile

VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
REGISTRY  ?= ghcr.io/stevenrbellis/unheaded

# All 25 service names
SERVICES := monad sophia wotan anamnesis shield unheaded-daemon \
            dashboard-backend dashboard-frontend protocol-api \
            trace-collector gateway service-discovery doom lich \
            captain micromanager timeguru architect developer \
            kanban lore busboy kingdom blackmage moatghost

BUILD_ARGS := \
  --build-arg VERSION=$(VERSION) \
  --build-arg COMMIT=$(COMMIT) \
  --build-arg BUILD_DATE=$(BUILD_DATE)

.PHONY: all build build-svc build-vllm push pull up-forge up-outpost down logs clean preflight status help

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

## Build vLLM/ROCm image
build-vllm:
	@echo "→ Building vLLM/ROCm image ($(VERSION))..."
	docker build $(BUILD_ARGS) \
	  -t unheaded-vllm-rocm:$(VERSION) \
	  -t unheaded-vllm-rocm:latest \
	  -f vllm-rocm/Dockerfile \
	  vllm-rocm/
	@echo "✓ vLLM/ROCm image built successfully"

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

## Push vLLM/ROCm image
push-vllm:
	@echo "→ Pushing vLLM/ROCm image to $(REGISTRY)..."
	docker tag unheaded-vllm-rocm:$(VERSION) $(REGISTRY)/vllm-rocm:$(VERSION)
	docker push $(REGISTRY)/vllm-rocm:$(VERSION)
	@echo "✓ vLLM/ROCm image pushed"

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

## Start Host-A (Forge) — full stack with all services
up-forge:
	@echo "→ Starting Forge (host-a)..."
	@if [ ! -d "hosts/host-a" ]; then \
	  echo "✗ hosts/host-a not found"; \
	  exit 1; \
	fi
	cd hosts/host-a && docker compose up -d
	@echo "✓ Forge (host-a) is running"
	@echo "  Use 'make status' to check service health"

## Start Host-B (Outpost) — minimal suite for edge compute
up-outpost:
	@echo "→ Starting Outpost (host-b)..."
	@if [ ! -d "hosts/host-b" ]; then \
	  echo "✗ hosts/host-b not found"; \
	  exit 1; \
	fi
	cd hosts/host-b && docker compose up -d
	@echo "✓ Outpost (host-b) is running"
	@echo "  Use 'make status' to check service health"

## Stop all services (host-a and host-b)
down:
	@echo "→ Stopping all services..."
	@cd hosts/host-a && docker compose down 2>/dev/null || true
	@cd hosts/host-b && docker compose down 2>/dev/null || true
	@echo "✓ All services stopped"

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
	  docker rmi unheaded-vllm-rocm:$(VERSION) unheaded-vllm-rocm:latest 2>/dev/null || true; \
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
	@echo "  make build-vllm               # Build vLLM/ROCm image"
	@echo "  make up-forge                 # Start host-a (full stack)"
	@echo "  make up-outpost               # Start host-b (minimal)"
	@echo "  make logs SERVICE=monad       # Follow logs for service"
	@echo "  make down                     # Stop all services"
	@echo "  make status                   # Show running containers"
	@echo ""
