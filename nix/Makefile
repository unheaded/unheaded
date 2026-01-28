# Unheaded NixOS Container Stack - Makefile
# Quick commands for building, testing, and deploying containers

.PHONY: help build test deploy clean check lint fmt

# Default target
help:
	@echo "Unheaded Container Stack"
	@echo "========================"
	@echo ""
	@echo "Build Commands:"
	@echo "  make build          - Build all containers"
	@echo "  make build-busboy   - Build busboy container"
	@echo "  make build-services - Build all agent services"
	@echo "  make build-apps     - Build kanban + dashboard apps"
	@echo ""
	@echo "Test Commands:"
	@echo "  make test           - Run all tests"
	@echo "  make test-unit      - Run unit tests only"
	@echo "  make test-integration - Run integration tests (requires LXD)"
	@echo "  make test-nix       - Run Nix VM tests"
	@echo ""
	@echo "Deploy Commands:"
	@echo "  make deploy         - Deploy all containers"
	@echo "  make start          - Start all containers"
	@echo "  make stop           - Stop all containers"
	@echo "  make restart        - Restart all containers"
	@echo "  make status         - Show container status"
	@echo ""
	@echo "Quality Commands:"
	@echo "  make check          - Run all checks (lint + build + test)"
	@echo "  make lint           - Lint Nix files"
	@echo "  make fmt            - Format Nix files"
	@echo ""
	@echo "Development:"
	@echo "  make shell          - Enter dev shell"
	@echo "  make clean          - Clean build artifacts"

# ==============================================================================
# BUILD TARGETS
# ==============================================================================

build: build-busboy build-services build-apps
	@echo "All containers built successfully!"

build-busboy:
	@echo "Building busboy..."
	nix build .#nixosConfigurations.busboy.config.system.build.toplevel

build-services: build-timeguru build-captain build-micromanager build-architect build-developer
	@echo "All services built!"

build-timeguru:
	@echo "Building timeguru..."
	nix build .#nixosConfigurations.timeguru.config.system.build.toplevel

build-captain:
	@echo "Building captain..."
	nix build .#nixosConfigurations.captain.config.system.build.toplevel

build-micromanager:
	@echo "Building micromanager..."
	nix build .#nixosConfigurations.micromanager.config.system.build.toplevel

build-architect:
	@echo "Building architect..."
	nix build .#nixosConfigurations.architect.config.system.build.toplevel

build-developer:
	@echo "Building developer..."
	nix build .#nixosConfigurations.developer.config.system.build.toplevel

build-apps: build-kanban build-dashboard
	@echo "Apps built!"

build-kanban:
	@echo "Building kanban..."
	nix build .#nixosConfigurations.kanban.config.system.build.toplevel

build-dashboard:
	@echo "Building dashboard..."
	nix build .#nixosConfigurations.dashboard.config.system.build.toplevel

# ==============================================================================
# TEST TARGETS
# ==============================================================================

test: test-unit test-integration
	@echo "All tests passed!"

test-unit:
	@echo "Running unit tests..."
	cd tests && go test -v -short

test-integration:
	@echo "Running integration tests (requires LXD)..."
	cd tests && go test -v -race

test-nix:
	@echo "Running Nix VM tests..."
	nix build .#hydraJobs.tests

# ==============================================================================
# DEPLOY TARGETS
# ==============================================================================

deploy:
	@echo "Deploying all containers..."
	nix run .#deploy

start:
	@echo "Starting all containers..."
	lxc start --all

stop:
	@echo "Stopping all containers..."
	lxc stop --all

restart:
	@echo "Restarting all containers..."
	lxc restart --all

status:
	@echo "Container status:"
	lxc list

# ==============================================================================
# QUALITY TARGETS
# ==============================================================================

check: lint build test
	@echo "All checks passed!"

lint:
	@echo "Linting Nix files..."
	nixpkgs-fmt --check .

fmt:
	@echo "Formatting Nix files..."
	nixpkgs-fmt .

# ==============================================================================
# DEVELOPMENT TARGETS
# ==============================================================================

shell:
	@echo "Entering dev shell..."
	nix develop

clean:
	@echo "Cleaning build artifacts..."
	rm -rf result result-*
	cd tests && go clean

# ==============================================================================
# MONITORING TARGETS
# ==============================================================================

logs-busboy:
	lxc exec unheaded-busboy -- journalctl -u busboy -f

logs-timeguru:
	lxc exec unheaded-timeguru -- journalctl -u timeguru -f

logs-all:
	@for c in busboy timeguru captain micromanager architect developer kanban dashboard; do \
		echo "=== $$c ===" ; \
		lxc exec unheaded-$$c -- journalctl -u $$c -n 20 ; \
	done

health-check:
	@echo "Checking health endpoints..."
	@curl -sf http://10.10.10.10:8080/health && echo "busboy: OK" || echo "busboy: FAIL"
	@curl -sf http://10.10.10.20:8000/health && echo "timeguru: OK" || echo "timeguru: FAIL"
	@curl -sf http://10.10.10.21:8001/health && echo "captain: OK" || echo "captain: FAIL"
	@curl -sf http://10.10.10.22:8002/health && echo "micromanager: OK" || echo "micromanager: FAIL"
	@curl -sf http://10.10.10.23:8003/health && echo "architect: OK" || echo "architect: FAIL"
	@curl -sf http://10.10.10.24:8004/health && echo "developer: OK" || echo "developer: FAIL"
	@curl -sf http://10.10.10.200:8080/health && echo "kanban: OK" || echo "kanban: FAIL"
	@curl -sf http://10.10.10.201:8081/health && echo "dashboard: OK" || echo "dashboard: FAIL"

metrics:
	@echo "Fetching metrics from all containers..."
	@for c in busboy timeguru captain micromanager architect developer kanban dashboard; do \
		echo "=== $$c metrics ===" ; \
		curl -s http://10.10.10.10:9100/metrics | head -20 ; \
	done

# ==============================================================================
# SECURITY TARGETS
# ==============================================================================

security-audit:
	@echo "Running security audit..."
	@for c in busboy timeguru captain micromanager architect developer kanban dashboard; do \
		echo "=== Auditing $$c ===" ; \
		lxc exec unheaded-$$c -- grep Seccomp /proc/1/status ; \
		lxc exec unheaded-$$c -- grep NoNewPrivs /proc/1/status ; \
	done

# ==============================================================================
# STATISTICS
# ==============================================================================

stats:
	@echo "Container Stack Statistics"
	@echo "=========================="
	@echo ""
	@echo "Lines of Code:"
	@find . -name "*.nix" -o -name "*.go" | xargs wc -l | tail -1
	@echo ""
	@echo "Files:"
	@echo "  Modules:    $(shell ls modules/*.nix | wc -l)"
	@echo "  Containers: $(shell ls containers/*.nix | wc -l)"
	@echo "  Packages:   $(shell ls packages/*.nix | wc -l)"
	@echo "  Tests:      $(shell ls tests/*.{go,nix} 2>/dev/null | wc -l)"
	@echo ""
	@echo "Containers Running:"
	@lxc list --format=csv | wc -l
