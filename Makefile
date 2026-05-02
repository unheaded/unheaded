.PHONY: all build test clean ebpf containers dev deploy docs help \
       ebpf-shield ebpf-hop ebpf-yaldabaoth ebpf-monad-cpu \
       build-monad-mbc pin-ebpf unpin-ebpf test-ebpf-compat \
       test-e2e-bpf deploy-down deploy-status deploy-lxd deploy-logs \
       deploy-restart sbom license-check \
       doom-smoke doom-build-check \
       ci-local ci-security sbom-verify

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

build-services: build-wotan build-timeguru build-captain build-architect build-micromanager build-monad build-sophia build-gateway build-zhen-rag ## Build all service binaries
	@echo "Building dashboard-backend..."
	@mkdir -p $(BINARY_DIR)
	cd cmd/dashboard-backend && go build $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS)" -o ../../$(BINARY_DIR)/dashboard-backend
	@echo "Building kanban-app..."
	cd cmd/kanban-app && go build $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS)" -o ../../$(BINARY_DIR)/kanban-app
	@echo "✓ Services built"

build-zhen-rag: ## Build zhen-rag (RAG glue: cs/vor + llama-server)
	@echo "Building zhen-rag..."
	@mkdir -p $(BINARY_DIR)
	cd cmd/zhen-rag && go build $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS)" -o ../../$(BINARY_DIR)/zhen-rag

build-zhen-agent: ## Build zhen-agent (Phase D-A: ReAct loop + Champion gate)
	@echo "Building zhen-agent..."
	@mkdir -p $(BINARY_DIR)
	cd cmd/zhen-agent && go build $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS)" -o ../../$(BINARY_DIR)/zhen-agent

coding-gate-ci: build-zhen-rag ## Run 5-seed CI gate; pre-merge required for any zhen-rag/RUBRIC/prompts change
	@echo "Running coding-gate 5-seed CI sweep..."
	@./scripts/probe-sanitization-check.sh
	@./scripts/coding-gate-ci.sh

coding-gate-sanitize: ## Verify probe finding-docs declare sanitized:true frontmatter
	@./scripts/probe-sanitization-check.sh

build-wotan: ## Build Wotan (Fae Chamber - Message Bus)
	@echo "🧚 Building Wotan..."
	@mkdir -p $(BINARY_DIR)
	cd services/wotan/cmd/wotan && go build $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS) -X main.version=$(VERSION)" -o ../../../../$(BINARY_DIR)/wotan

build-timeguru: ## Build Timeguru (Oracle's Antre - Timeline)
	@echo "⌛ Building Timeguru..."
	@mkdir -p $(BINARY_DIR)
	cd services/timeguru/cmd/timeguru && go build $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS) -X main.version=$(VERSION)" -o ../../../../$(BINARY_DIR)/timeguru

build-captain: ## Build Captain (Commander's Quarters - Vision)
	@echo "👑 Building Captain..."
	@mkdir -p $(BINARY_DIR)
	cd services/captain/cmd/captain && go build $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS) -X main.version=$(VERSION)" -o ../../../../$(BINARY_DIR)/captain

build-architect: ## Build Architect (Sage's Lair - ADRs)
	@echo "🏗️ Building Architect..."
	@mkdir -p $(BINARY_DIR)
	cd services/architect/cmd/architect && go build $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS) -X main.version=$(VERSION)" -o ../../../../$(BINARY_DIR)/architect

build-micromanager: ## Build Micromanager (War Room - Tasks)
	@echo "📋 Building Micromanager..."
	@mkdir -p $(BINARY_DIR)
	cd services/micromanager/cmd/micromanager && go build $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS) -X main.version=$(VERSION)" -o ../../../../$(BINARY_DIR)/micromanager

build-monad: ## Build Monad (Unified State Management)
	@echo "Building Monad..."
	@mkdir -p $(BINARY_DIR)
	cd cmd/monad && go build $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS) -X main.version=$(VERSION)" -o ../../$(BINARY_DIR)/monad

build-sophia: ## Build Sophia (Knowledge Graph)
	@echo "Building Sophia..."
	@mkdir -p $(BINARY_DIR)
	cd cmd/sophia && go build $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS) -X main.version=$(VERSION)" -o ../../$(BINARY_DIR)/sophia

build-gateway: ## Build Gateway (API Gateway)
	@echo "Building Gateway..."
	@mkdir -p $(BINARY_DIR)
	cd services/gateway/cmd && go build $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS) -X main.version=$(VERSION)" -o ../../../$(BINARY_DIR)/gateway

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

##@ eBPF Protocol Foundation (individual targets)

ebpf-shield: ## Build shield-ebpf only (Layer 0 boundary)
	cd $(EBPF_DIR) && cargo build $(CARGO_BUILD_FLAGS) --target=bpfel-unknown-none -Z build-std=core -p shield-ebpf

ebpf-hop: ## Build hop-ebpf only (Layer 1 per-hop processor)
	cd $(EBPF_DIR) && cargo build $(CARGO_BUILD_FLAGS) --target=bpfel-unknown-none -Z build-std=core -p hop-ebpf

ebpf-yaldabaoth: ## Build yaldabaoth-ebpf only (chaos injection)
	cd $(EBPF_DIR) && cargo build $(CARGO_BUILD_FLAGS) --target=bpfel-unknown-none -Z build-std=core -p yaldabaoth-ebpf

ebpf-monad-cpu: ## Build monad-cpu-ebpf only (Doom PoC VM)
	cd $(EBPF_DIR) && cargo build $(CARGO_BUILD_FLAGS) --target=bpfel-unknown-none -Z build-std=core -p monad-cpu-ebpf

build-monad-mbc: ## Build monad-mbc assembler/translator
	@echo "Building monad-mbc..."
	cd crates/monad-mbc && cargo build $(CARGO_BUILD_FLAGS)
	@echo "✓ monad-mbc built"

pin-ebpf: ## Load and pin eBPF programs to /sys/fs/bpf/unheaded/
	@echo "Loading and pinning eBPF programs..."
	@mkdir -p /sys/fs/bpf/unheaded
	sudo ./scripts/load-ebpf.sh
	@echo "✓ eBPF programs pinned"

unpin-ebpf: ## Remove pinned eBPF maps from /sys/fs/bpf/unheaded/
	@echo "Removing pinned eBPF maps..."
	sudo rm -rf /sys/fs/bpf/unheaded/
	@echo "✓ eBPF pins cleared"

test-ebpf-compat: ## Verify eBPF workspace builds for host target (type-checks only)
	@echo "Type-checking eBPF workspace for host target..."
	cd $(EBPF_DIR) && cargo check --target=$(shell rustup show active-toolchain | grep -oP 'x86_64[^ ]+' || echo "x86_64-unknown-linux-gnu")
	@echo "✓ eBPF type-check passed"

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
	cd crates/monad-mbc && cargo test

test-e2e: ## Run E2E integration tests (auth, pipeline, security, message delivery)
	@echo "Running E2E integration tests..."
	go test -v -race -count=1 -timeout 300s ./tests/e2e/...
	@echo "✓ E2E tests passed"

test-e2e-bpf: ## Run BPF → Dashboard E2E integration tests (requires root for real BPF)
	@echo "Running BPF → Dashboard E2E tests..."
	go test -v -race -count=1 -run TestE2E_BPFDashboard ./cmd/dashboard-backend/internal/ebpf/
	@echo "✓ BPF E2E tests passed"

test-security: ## Run Lich security campaigns (requires running services)
	@echo "Running Lich security campaigns..."
	go run ./cmd/lich-security -gateway http://localhost:21000 -report /tmp/lich-report.md
	@echo "✓ Security campaigns complete. Report: /tmp/lich-report.md"

race-test: ## Run Go tests with race detection only
	@echo "Running race detection tests..."
	go test -race -count=1 -timeout 300s ./...
	@echo "✓ No races detected"

bench: ## Run benchmarks
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

##@ DOOM Testing

doom-smoke: ## Run DOOM full lifecycle smoke test (requires root + BPF)
	sudo ./scripts/doom-smoke.sh

doom-build-check: ## Verify DOOM build artifacts for known bug regressions
	./scripts/doom-build-check.sh

##@ Containers

containers: ## Build all NixOS container images
	@echo "Building NixOS containers..."
	cd $(NIX_DIR) && nix build .#containers --out-link result-containers
	@echo "✓ Containers built"

container-wotan: ## Build Wotan container
	cd $(NIX_DIR) && nix build .#containers.wotan

container-gateway: ## Build Gateway container
	cd $(NIX_DIR) && nix build .#containers.gateway

##@ Development

dev: ## Run development environment (docker-compose)
	@echo "Starting development environment..."
	docker compose up -d
	@echo "Dashboard: http://localhost:20000"
	@echo "Kanban: http://localhost:20001"

dev-down: ## Stop development environment
	docker compose down

dev-logs: ## Show development logs
	docker compose logs -f

##@ Docker (Kingdom Services)

docker: docker-build ## Build all Docker images

docker-build: ## Build Docker images
	@echo "🐳 Building Kingdom Docker images..."
	docker compose build --build-arg VERSION=$(VERSION)
	@echo "✓ Docker images built"

docker-up: ## Start Kingdom services
	@echo "🚀 Raising the Kingdom..."
	docker compose up -d
	docker compose ps

docker-down: ## Stop Kingdom services
	@echo "🔻 Kingdom rests..."
	docker compose down

docker-restart: ## Restart Kingdom services
	docker compose restart

docker-clean: ## Clean Docker resources
	docker compose down -v --remove-orphans

docker-wotan: ## Build only Wotan image
	docker build --target wotan -t unheaded/wotan:$(VERSION) .

docker-timeguru: ## Build only Timeguru image
	docker build --target timeguru -t unheaded/timeguru:$(VERSION) .

docker-cuirass: ## Build only Cuirass image
	docker build --target cuirass -t unheaded/cuirass:$(VERSION) .

##@ Deployment

deploy: build test-go ## Deploy Unheaded Kingdom (build, test, compose up, health check)
	@echo "Deploying Unheaded Kingdom..."
	docker compose up -d --build
	@echo "Waiting for services to become healthy..."
	./scripts/wait-for-healthy.sh
	@echo "Kingdom deployed."

deploy-down: ## Stop Unheaded Kingdom (compose down)
	docker compose down

deploy-status: ## Show Unheaded Kingdom service status
	docker compose ps

deploy-lxd: ## Deploy alpha to LXD containers (production-style)
	@echo "Deploying Unheaded alpha to LXD..."
	sudo ./scripts/deploy-alpha.sh

deploy-logs: ## Tail deployment logs
	docker compose logs -f

deploy-restart: ## Restart all deployed services
	docker compose restart

deploy-health: ## Verify all deployed services are healthy
	@echo "Checking service health..."
	@for svc in wotan timeguru captain architect micromanager monad sophia cuirass; do \
		printf "%-16s " "$$svc:"; \
		curl -sf http://localhost:$$(docker compose port $$svc 2>/dev/null | cut -d: -f2)/health > /dev/null 2>&1 && echo "healthy" || echo "UNHEALTHY"; \
	done

deploy-rollback: ## Rollback to previous Docker image versions
	@echo "Rolling back to previous deployment..."
	docker compose down
	docker compose up -d --no-build
	@echo "Rollback complete. Run 'make deploy-health' to verify."

##@ Operation Three Crowns (Dual-Host Runtime Validation)

three-crowns: ## Run full Operation Three Crowns (Docker + LXD + containerd, both hosts)
	@bash scripts/three-crowns.sh

three-crowns-docker: ## Run Three Crowns Phase 2 only (Docker runtime)
	@bash scripts/three-crowns.sh --phase 2

three-crowns-lxd: ## Run Three Crowns Phase 3 only (LXD runtime)
	@bash scripts/three-crowns.sh --phase 3

three-crowns-nixos: ## Run Three Crowns Phase 3b only (NixOS on LXD)
	@bash scripts/three-crowns.sh --phase 3b

three-crowns-containerd: ## Run Three Crowns Phase 4 only (containerd runtime)
	@bash scripts/three-crowns.sh --phase 4

three-crowns-report: ## Print last Three Crowns report
	@bash scripts/three-crowns.sh --report

three-crowns-dry: ## Dry-run Three Crowns (show what would execute)
	@bash scripts/three-crowns.sh --dry-run

install-nerdctl: ## Install nerdctl + CNI plugins for containerd
	@bash scripts/install-nerdctl.sh

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
	rm -f e2e.test kanban-app kanban-app.test runtime.test cover.out
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

security: ## Run Go security audit (gosec)
	@echo "Running security audit..."
	@which gosec > /dev/null || (echo "Installing gosec..." && go install github.com/securego/gosec/v2/cmd/gosec@v2.21.0)
	gosec ./...

##@ SBOM & Compliance

sbom: ## Generate SBOM in multiple formats (CycloneDX, SPDX, module list, audit)
	@echo "Generating SBOM..."
	./scripts/generate-sbom.sh $(VERSION)
	@echo "✓ SBOM generation complete. See sbom/"

license-check: ## Check Go and Rust dependencies for permitted licenses
	@echo "Checking Go licenses..."
	@which go-licenses > /dev/null || (echo "Installing go-licenses..." && go install github.com/google/go-licenses@latest)
	go-licenses check ./... --allowed_licenses=MIT,Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC
	@echo "✓ Go license check passed"
	@if [ -d "ebpf" ] && [ -f "ebpf/Cargo.toml" ]; then \
		echo "Checking Rust licenses (ebpf)..."; \
		cd ebpf && cargo license --all-features > /dev/null && echo "✓ Rust licenses OK" || true; \
		cd ..; \
	fi

ci-local: ## Run full CI pipeline locally
	@echo "Running local CI pipeline..."
	@chmod +x scripts/ci-local.sh
	scripts/ci-local.sh

ci-security: ## Run security scans only (gosec + govulncheck + GPL boundary)
	@echo "Running security scans..."
	@which gosec > /dev/null || (echo "Installing gosec..." && go install github.com/securego/gosec/v2/cmd/gosec@v2.21.0)
	gosec ./...
	@which govulncheck > /dev/null || (echo "Installing govulncheck..." && go install golang.org/x/vuln/cmd/govulncheck@latest)
	govulncheck ./...
	@chmod +x scripts/verify-gpl-boundary.sh
	scripts/verify-gpl-boundary.sh
	@echo "✓ All security scans passed"

sbom-verify: ## Run GPL boundary verification
	@echo "Running GPL boundary verification..."
	@chmod +x scripts/verify-gpl-boundary.sh
	scripts/verify-gpl-boundary.sh

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
