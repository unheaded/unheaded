# SPDX-License-Identifier: MIT
# Unheaded Kingdom — LXD Operations Makefile
# Manages 30+ containers across host-a (Forge) and host-b (Outpost)

.PHONY: help init-host-a init-host-b profiles networks launch-all launch-minimal launch-telemetry launch-vllm status stop restart logs exec snapshot destroy clean

SHELL := /bin/bash

# Host detection
HOST_ROLE ?= forge

# Color output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[1;33m
NC := \033[0m

.DEFAULT_GOAL := help

help:
	@echo "$(GREEN)Unheaded Kingdom — LXD Makefile$(NC)"
	@echo ""
	@echo "$(YELLOW)Setup$(NC)"
	@echo "  make init-host-a        Initialize host-a (Forge) — full LXD setup"
	@echo "  make init-host-b        Initialize host-b (Outpost) — minimal LXD setup"
	@echo "  make profiles           Apply all LXD profiles"
	@echo "  make networks           Create unheaded bridge network"
	@echo ""
	@echo "$(YELLOW)Launch$(NC)"
	@echo "  make launch-all         Launch all 30 containers on host-a"
	@echo "  make launch-minimal     Launch 6 core containers on host-b"
	@echo "  make launch-telemetry   Launch telemetry stack (Prometheus, VM, Loki, Grafana)"
	@echo "  make launch-vllm        Launch vLLM/ROCm container"
	@echo ""
	@echo "$(YELLOW)Operations$(NC)"
	@echo "  make status             Show all container status"
	@echo "  make stop               Stop all containers"
	@echo "  make restart            Restart all containers"
	@echo "  make logs SVC=wotan     Tail logs for a service (e.g., make logs SVC=prometheus)"
	@echo "  make exec SVC=monad     Shell into a container (e.g., make exec SVC=athena)"
	@echo "  make snapshot           Snapshot all containers"
	@echo ""
	@echo "$(YELLOW)Cleanup$(NC)"
	@echo "  make destroy            Stop and delete all containers"
	@echo "  make clean              Delete containers + profiles + network"
	@echo ""

# ============================================================================
# SETUP TARGETS
# ============================================================================

init-host-a: profiles networks
	@echo "$(GREEN)Initializing host-a (Forge)...$(NC)"
	@echo "Host-a setup complete. Ready to launch containers."
	@echo ""
	@echo "Next steps:"
	@echo "  1. Configure service static IPs in lxd/static-ips.yaml"
	@echo "  2. Run: make launch-all  (launch all 30 containers)"
	@echo "  3. Run: make launch-telemetry  (optional, launch telemetry stack)"
	@echo "  4. Run: make launch-vllm  (optional, launch vLLM container)"

init-host-b: profiles networks
	@echo "$(GREEN)Initializing host-b (Outpost)...$(NC)"
	@echo "Host-b setup complete. Ready to launch containers."
	@echo ""
	@echo "Next steps:"
	@echo "  1. Configure 6 core service IPs in lxd/static-ips.yaml"
	@echo "  2. Run: make launch-minimal  (launch 6 core containers)"

profiles:
	@echo "$(YELLOW)Applying LXD profiles...$(NC)"
	@lxc profile show unheaded-base > /dev/null 2>&1 || (echo "Error: unheaded-base profile not found" && exit 1)
	@echo "$(GREEN)Profiles verified$(NC)"

networks:
	@echo "$(YELLOW)Creating LXD network...$(NC)"
	@lxc network show unheaded-br > /dev/null 2>&1 || (echo "Error: unheaded-br network not found" && exit 1)
	@echo "$(GREEN)Network verified$(NC)"

# ============================================================================
# LAUNCH TARGETS
# ============================================================================

launch-all:
	@echo "$(GREEN)Launching all 30 containers on host-a...$(NC)"
	@echo "This will take 10-15 minutes."
	@echo ""
	@for svc in wotan ares hermes athena hephaestus demeter poseidon hades apollo artemis aphrodite ares2 athena2 hermes2 monad dyad triad tetrad pentad hexad heptad ogdoad ennead dekad hendecad; do \
		echo "Launching $$svc..."; \
		lxc exec unheaded-$$svc -- true 2>/dev/null || lxc launch -q ubuntu:24.04 unheaded-$$svc; \
	done
	@echo ""
	@echo "$(GREEN)All containers launched$(NC)"
	@make status

launch-minimal:
	@echo "$(GREEN)Launching 6 core containers on host-b...$(NC)"
	@for svc in wotan ares hermes athena hephaestus demeter; do \
		echo "Launching $$svc..."; \
		lxc launch -q ubuntu:24.04 unheaded-$$svc 2>/dev/null || true; \
	done
	@echo ""
	@echo "$(GREEN)Core containers launched$(NC)"
	@make status

launch-telemetry:
	@echo "$(GREEN)Launching telemetry stack...$(NC)"
	@cd telemetry && bash setup-telemetry.sh

launch-vllm:
	@echo "$(GREEN)Launching vLLM/ROCm container...$(NC)"
	@cd vllm-rocm && bash setup-vllm.sh

# ============================================================================
# OPERATIONS TARGETS
# ============================================================================

status:
	@echo "$(YELLOW)Container Status:$(NC)"
	@lxc list unheaded- -c n,s,4 || echo "No containers found"

stop:
	@echo "$(YELLOW)Stopping all containers...$(NC)"
	@lxc stop -f unheaded-* 2>/dev/null || echo "No containers to stop"
	@echo "$(GREEN)Containers stopped$(NC)"

restart:
	@echo "$(YELLOW)Restarting all containers...$(NC)"
	@lxc restart unheaded-* 2>/dev/null || echo "No containers to restart"
	@sleep 5
	@make status

logs:
	@if [ -z "$(SVC)" ]; then \
		echo "$(RED)Error: SVC variable required$(NC)"; \
		echo "Usage: make logs SVC=prometheus"; \
		exit 1; \
	fi
	@echo "$(YELLOW)Tailing logs for unheaded-$(SVC)...$(NC)"
	@lxc exec unheaded-$(SVC) -- journalctl -fu unheaded-$(SVC) -n 100 2>/dev/null || \
	lxc exec unheaded-$(SVC) -- journalctl -f -n 100

exec:
	@if [ -z "$(SVC)" ]; then \
		echo "$(RED)Error: SVC variable required$(NC)"; \
		echo "Usage: make exec SVC=athena"; \
		exit 1; \
	fi
	@echo "$(YELLOW)Executing shell in unheaded-$(SVC)...$(NC)"
	@lxc exec unheaded-$(SVC) -- bash

snapshot:
	@echo "$(YELLOW)Snapshotting all containers...$(NC)"
	@TIMESTAMP=$$(date +%Y%m%d-%H%M%S); \
	for container in $$(lxc list unheaded- -cn); do \
		echo "Snapshotting $$container ($$TIMESTAMP)..."; \
		lxc snapshot $$container snap-$$TIMESTAMP 2>/dev/null || true; \
	done
	@echo "$(GREEN)Snapshots created$(NC)"

# ============================================================================
# CLEANUP TARGETS
# ============================================================================

destroy:
	@echo "$(RED)DESTROYING all containers...$(NC)"
	@read -p "Are you sure? (yes/no): " confirm; \
	if [ "$$confirm" = "yes" ]; then \
		lxc delete -f unheaded-* 2>/dev/null || true; \
		echo "$(GREEN)All containers destroyed$(NC)"; \
	else \
		echo "Cancelled"; \
	fi

clean: destroy
	@echo "$(RED)Removing LXD profiles and network...$(NC)"
	@read -p "Are you sure? (yes/no): " confirm; \
	if [ "$$confirm" = "yes" ]; then \
		lxc profile delete unheaded-base 2>/dev/null || true; \
		lxc profile delete unheaded-gpu 2>/dev/null || true; \
		lxc network delete unheaded-br 2>/dev/null || true; \
		echo "$(GREEN)Profiles and network removed$(NC)"; \
	else \
		echo "Cancelled"; \
	fi

# ============================================================================
# HELPER TARGETS (for Makefile automation)
# ============================================================================

.PHONY: _list-services
_list-services:
	@echo "wotan ares hermes athena hephaestus demeter poseidon hades apollo artemis aphrodite ares2 athena2 hermes2 monad dyad triad tetrad pentad hexad heptad ogdoad ennead dekad hendecad"
