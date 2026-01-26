.PHONY: all build test clean ebpf containers dev deploy docs help

# Build configuration
BINARY_DIR := bin
EBPF_DIR := ebpf
NIX_DIR := nix
DOCS_DIR := docs

# Go build flags
GO_BUILD_FLAGS := -v -trimpath
GO_LDFLAGS := -s -w

# Rust/eBPF build flags
CARGO_BUILD_FLAGS := --release

# Container registry (change for production)
CONTAINER_REGISTRY := ghcr.io/unheaded

# Version (read from git or default)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

all: build ebpf ## Build everything

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

##@ Build

build: build-daemon build-services ## Build all Go binaries
	@echo "✓ All binaries built"

build-daemon: ## Build unheaded-daemon
	@echo "Building unheaded-daemon..."
	@mkdir -p $(BINARY_DIR)
	cd cmd/unheaded-daemon && go build $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS) -X main.Version=$(VERSION)" -o ../../$(BINARY_DIR)/unheaded-daemon

build-services: ## Build all service binaries
	@echo "Building dashboard-backend..."
	@mkdir -p $(BINARY_DIR)
	cd cmd/dashboard-backend && go build $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS)" -o ../../$(BINARY_DIR)/dashboard-backend
	@echo "Building kanban-app..."
	cd cmd/kanban-app && go build $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS)" -o ../../$(BINARY_DIR)/kanban-app
	@echo "✓ Services built"

build-trace-collector: ## Build trace-collector (Rust)
	@echo "Building trace-collector (Rust)..."
	@mkdir -p $(BINARY_DIR)
	cd cmd/trace-collector && cargo build $(CARGO_BUILD_FLAGS)
	cp cmd/trace-collector/target/release/trace-collector $(BINARY_DIR)/
	@echo "✓ trace-collector built"

ebpf: ## Build eBPF programs (Rust)
	@echo "Building eBPF programs..."
	cd $(EBPF_DIR) && cargo build $(CARGO_BUILD_FLAGS) --target=bpfel-unknown-none
	@echo "✓ eBPF programs built"

##@ Testing

test: test-go test-rust ## Run all tests
	@echo "✓ All tests passed"

test-go: ## Run Go tests
	@echo "Running Go tests..."
	go test -v -race -cover ./...

test-rust: ## Run Rust tests
	@echo "Running Rust tests..."
	cd $(EBPF_DIR) && cargo test
	cd cmd/trace-collector && cargo test

bench: ## Run benchmarks
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

##@ Containers

containers: ## Build all NixOS container images
	@echo "Building NixOS containers..."
	cd $(NIX_DIR) && nix build .#containers --out-link result-containers
	@echo "✓ Containers built"

container-busboy: ## Build Busboy container
	cd $(NIX_DIR) && nix build .#containers.busboy

container-gateway: ## Build Gateway container
	cd $(NIX_DIR) && nix build .#containers.gateway

##@ Development

dev: ## Run development environment (docker-compose)
	@echo "Starting development environment..."
	docker-compose up -d
	@echo "Dashboard: http://localhost:8080"
	@echo "Kanban: http://localhost:8081"

dev-down: ## Stop development environment
	docker-compose down

dev-logs: ## Show development logs
	docker-compose logs -f

##@ Deployment

deploy: ## Deploy alpha to host
	@echo "Deploying Unheaded alpha..."
	sudo ./scripts/deploy-alpha.sh

setup-host: ## Setup host environment (LXD, networking, etc)
	@echo "Setting up host..."
	sudo ./scripts/setup-host.sh

load-ebpf: ## Load eBPF programs
	@echo "Loading eBPF programs..."
	sudo ./scripts/load-ebpf.sh

##@ Documentation

docs: ## Generate documentation
	@echo "Generating documentation..."
	cd $(DOCS_DIR) && make

docs-serve: ## Serve documentation locally
	cd $(DOCS_DIR) && make serve

##@ Cleanup

clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	rm -rf $(BINARY_DIR)
	rm -rf $(EBPF_DIR)/target
	rm -rf cmd/trace-collector/target
	cd $(NIX_DIR) && rm -f result-*
	@echo "✓ Clean complete"

clean-containers: ## Remove all LXD containers
	@echo "Removing LXD containers..."
	sudo lxc list --format json | jq -r '.[].name' | grep '^unheaded-' | xargs -r sudo lxc delete -f

##@ Release

release: clean all test ## Build release artifacts
	@echo "Building release $(VERSION)..."
	@mkdir -p releases/$(VERSION)
	cp $(BINARY_DIR)/* releases/$(VERSION)/
	cd releases && tar czf unheaded-$(VERSION).tar.gz $(VERSION)
	@echo "✓ Release $(VERSION) ready: releases/unheaded-$(VERSION).tar.gz"

##@ Utilities

fmt: ## Format all code
	@echo "Formatting Go code..."
	go fmt ./...
	@echo "Formatting Rust code..."
	cd $(EBPF_DIR) && cargo fmt
	cd cmd/trace-collector && cargo fmt

lint: ## Lint all code
	@echo "Linting Go code..."
	golangci-lint run ./...
	@echo "Linting Rust code..."
	cd $(EBPF_DIR) && cargo clippy
	cd cmd/trace-collector && cargo clippy

deps: ## Download dependencies
	@echo "Downloading Go dependencies..."
	go mod download
	@echo "Downloading Rust dependencies..."
	cd $(EBPF_DIR) && cargo fetch
	cd cmd/trace-collector && cargo fetch

tidy: ## Tidy dependencies
	go mod tidy

##@ Demo

demo: ## Run full demo
	@echo "Running Unheaded alpha demo..."
	./scripts/demo-kanban.sh

status: ## Show deployment status
	@echo "=== Unheaded Status ==="
	@echo ""
	@echo "LXD Containers:"
	@sudo lxc list | grep unheaded || echo "No containers running"
	@echo ""
	@echo "eBPF Programs:"
	@sudo bpftool prog list | grep unheaded || echo "No eBPF programs loaded"
	@echo ""
	@echo "Services:"
	@systemctl is-active unheaded-daemon 2>/dev/null || echo "unheaded-daemon: not running"

.DEFAULT_GOAL := help
