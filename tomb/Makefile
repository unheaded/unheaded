# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# tomb/Makefile -- Tomb of Knowledge build and provisioning targets
#
# Usage:
#   make provision          Full provisioning (all layers + hardening)
#   make boot               Boot the Tomb VM via QEMU
#   make shutdown           Clean shutdown of the Tomb VM
#   make status             Check VM and service status
#   make deploy-lich        Deploy Layer 2: Lich adversary framework
#   make deploy-grimoire    Deploy Layer 3: Grimoire knowledge base
#   make deploy-oracle      Deploy Layer 4: Oracle LLM
#   make deploy-dark-mirror Deploy Layer 5: Dark Mirror observability
#   make harden             Run Phase 13 security hardening
#   make test               Run integration tests
#   make ansible-full       Run full Ansible deployment
#   make ansible-check      Ansible dry-run (check mode)

SHELL := /bin/bash
.DEFAULT_GOAL := help

# ============================================================================
# Configuration
# ============================================================================
TOMB_TARGET    ?= 192.168.13.6
TOMB_USER      ?= kali
TOMB_SSH_KEY   ?= $(HOME)/.ssh/id_tomb
HOST_IP        ?= 192.168.13.2
BRIDGE_IP      ?= 192.168.13.5

# QEMU settings
QEMU_BIN       ?= qemu-system-x86_64
QEMU_MEM       ?= 8G
QEMU_CPUS      ?= 4
QEMU_DISK      ?= $(HOME)/vms/tomb-kali.qcow2
QEMU_ISO       ?= $(HOME)/isos/kali-linux-2024.4-live-amd64.iso
QEMU_PID       ?= /tmp/tomb-qemu.pid
QEMU_MONITOR   ?= /tmp/tomb-qemu.monitor

# Paths
TOMB_DIR       := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
PROVISION_SH   := $(TOMB_DIR)/provision.sh
ANSIBLE_DIR    := $(TOMB_DIR)/ansible
INVENTORY      := $(ANSIBLE_DIR)/inventory.yml
LOG_DIR        := $(TOMB_DIR)/logs
PACKAGES_DIR   := $(TOMB_DIR)/packages

# SSH
SSH_OPTS       := -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
                  -o ConnectTimeout=10 -o BatchMode=yes -o LogLevel=ERROR \
                  -i $(TOMB_SSH_KEY)

# Ansible
ANSIBLE_OPTS   := -i $(INVENTORY) --extra-vars "tomb_target=$(TOMB_TARGET)"
ANSIBLE_PLAY   := ansible-playbook $(ANSIBLE_OPTS)

# ============================================================================
# Help
# ============================================================================
.PHONY: help
help: ## Show this help
	@echo ""
	@echo "  Tomb of Knowledge -- Makefile Targets"
	@echo "  ======================================"
	@echo ""
	@echo "  VM Management:"
	@grep -E '^(boot|shutdown|console|snapshot|status):' $(MAKEFILE_LIST) | \
		awk -F ':.*?## ' '{printf "    %-22s %s\n", $$1, $$2}'
	@echo ""
	@echo "  Provisioning (provision.sh):"
	@grep -E '^(provision|deploy-|harden|setup-base):' $(MAKEFILE_LIST) | \
		awk -F ':.*?## ' '{printf "    %-22s %s\n", $$1, $$2}'
	@echo ""
	@echo "  Ansible Playbooks:"
	@grep -E '^ansible-' $(MAKEFILE_LIST) | \
		awk -F ':.*?## ' '{printf "    %-22s %s\n", $$1, $$2}'
	@echo ""
	@echo "  Testing:"
	@grep -E '^test' $(MAKEFILE_LIST) | \
		awk -F ':.*?## ' '{printf "    %-22s %s\n", $$1, $$2}'
	@echo ""
	@echo "  Utilities:"
	@grep -E '^(ssh|logs|clean|stage-packages):' $(MAKEFILE_LIST) | \
		awk -F ':.*?## ' '{printf "    %-22s %s\n", $$1, $$2}'
	@echo ""

# ============================================================================
# VM Management
# ============================================================================
.PHONY: boot
boot: ## Boot the Tomb VM via QEMU (serial console, air-gapped)
	@echo "[INFO] Booting Tomb VM..."
	@echo "[INFO] Disk:   $(QEMU_DISK)"
	@echo "[INFO] Memory: $(QEMU_MEM)"
	@echo "[INFO] CPUs:   $(QEMU_CPUS)"
	@echo "[INFO] Exit:   Ctrl+A X"
	@echo ""
	@if [ ! -f "$(QEMU_DISK)" ]; then \
		echo "[WARN] Disk image not found: $(QEMU_DISK)"; \
		echo "[INFO] Create with: qemu-img create -f qcow2 $(QEMU_DISK) 30G"; \
		exit 1; \
	fi
	$(QEMU_BIN) \
		-name "tomb-of-knowledge" \
		-machine type=q35,accel=kvm \
		-cpu host \
		-smp $(QEMU_CPUS) \
		-m $(QEMU_MEM) \
		-drive file=$(QEMU_DISK),format=qcow2,if=virtio \
		-netdev tap,id=net0,ifname=tap-tomb,script=no,downscript=no \
		-device virtio-net-pci,netdev=net0,mac=52:54:00:13:00:06 \
		-nographic \
		-serial mon:stdio \
		-pidfile $(QEMU_PID) \
		-monitor unix:$(QEMU_MONITOR),server,nowait

.PHONY: boot-slirp
boot-slirp: ## Boot with user networking (no tap, simpler setup)
	@echo "[INFO] Booting Tomb VM (slirp networking)..."
	$(QEMU_BIN) \
		-name "tomb-of-knowledge" \
		-machine type=q35,accel=kvm \
		-cpu host \
		-smp $(QEMU_CPUS) \
		-m $(QEMU_MEM) \
		-drive file=$(QEMU_DISK),format=qcow2,if=virtio \
		-netdev user,id=net0,hostfwd=tcp:$(HOST_IP):2222-:22,net=192.168.13.0/24,host=192.168.13.5,dhcpstart=192.168.13.6 \
		-device virtio-net-pci,netdev=net0,mac=52:54:00:13:00:06 \
		-nographic \
		-serial mon:stdio \
		-pidfile $(QEMU_PID)

.PHONY: shutdown
shutdown: ## Clean shutdown of the Tomb VM
	@echo "[INFO] Shutting down Tomb VM..."
	@if [ -f "$(QEMU_PID)" ]; then \
		ssh $(SSH_OPTS) $(TOMB_USER)@$(TOMB_TARGET) "sudo shutdown -h now" 2>/dev/null || true; \
		echo "[INFO] Waiting for VM to power off..."; \
		sleep 5; \
		if [ -f "$(QEMU_PID)" ] && kill -0 $$(cat $(QEMU_PID)) 2>/dev/null; then \
			echo "[WARN] VM still running, sending SIGTERM..."; \
			kill $$(cat $(QEMU_PID)) 2>/dev/null || true; \
		fi; \
		rm -f $(QEMU_PID); \
		echo "[  OK] Tomb VM shut down"; \
	else \
		echo "[INFO] No PID file found -- VM may not be running"; \
		ssh $(SSH_OPTS) $(TOMB_USER)@$(TOMB_TARGET) "sudo shutdown -h now" 2>/dev/null || true; \
	fi

.PHONY: console
console: ## Attach to QEMU monitor console
	@if [ -S "$(QEMU_MONITOR)" ]; then \
		echo "[INFO] Connecting to QEMU monitor..."; \
		socat - UNIX-CONNECT:$(QEMU_MONITOR); \
	else \
		echo "[WARN] Monitor socket not found: $(QEMU_MONITOR)"; \
	fi

.PHONY: snapshot
snapshot: ## Take a QEMU snapshot of the running VM
	@echo "[INFO] Taking VM snapshot..."
	@if [ -S "$(QEMU_MONITOR)" ]; then \
		echo "savevm tomb-$$(date +%Y%m%d-%H%M%S)" | socat - UNIX-CONNECT:$(QEMU_MONITOR); \
		echo "[  OK] Snapshot saved"; \
	else \
		echo "[WARN] Cannot snapshot -- QEMU monitor not available"; \
		echo "[INFO] Alternative: qemu-img snapshot -c snap1 $(QEMU_DISK)"; \
	fi

.PHONY: status
status: ## Check VM and service status
	@echo ""
	@echo "  Tomb of Knowledge -- Status"
	@echo "  ============================"
	@echo ""
	@echo "  VM:"
	@if [ -f "$(QEMU_PID)" ] && kill -0 $$(cat $(QEMU_PID)) 2>/dev/null; then \
		echo "    QEMU Process: RUNNING (PID $$(cat $(QEMU_PID)))"; \
	else \
		echo "    QEMU Process: NOT RUNNING"; \
	fi
	@echo ""
	@echo "  Connectivity:"
	@ping -c 1 -W 2 $(TOMB_TARGET) >/dev/null 2>&1 \
		&& echo "    Ping $(TOMB_TARGET): REACHABLE" \
		|| echo "    Ping $(TOMB_TARGET): UNREACHABLE"
	@ssh $(SSH_OPTS) $(TOMB_USER)@$(TOMB_TARGET) "echo alive" >/dev/null 2>&1 \
		&& echo "    SSH:             CONNECTED" \
		|| echo "    SSH:             FAILED"
	@echo ""
	@echo "  Services (via SSH):"
	@ssh $(SSH_OPTS) $(TOMB_USER)@$(TOMB_TARGET) "\
		for svc in ollama oracle-daemon tomb-prometheus tomb-grafana tomb-loki tomb-firewall; do \
			state=\$$(systemctl is-active \$$svc 2>/dev/null || echo 'unknown'); \
			printf '    %-22s %s\n' \"\$$svc:\" \"\$$state\"; \
		done" 2>/dev/null || echo "    (could not query services via SSH)"
	@echo ""
	@echo "  Layers:"
	@ssh $(SSH_OPTS) $(TOMB_USER)@$(TOMB_TARGET) "\
		for d in lich grimoire oracle dark-mirror captures reports; do \
			if [ -d /opt/tomb/\$$d ]; then \
				count=\$$(find /opt/tomb/\$$d -type f 2>/dev/null | wc -l); \
				printf '    %-22s %s files\n' \"/opt/tomb/\$$d:\" \"\$$count\"; \
			else \
				printf '    %-22s %s\n' \"/opt/tomb/\$$d:\" 'MISSING'; \
			fi; \
		done" 2>/dev/null || echo "    (could not query layers via SSH)"
	@echo ""

# ============================================================================
# Provisioning (provision.sh)
# ============================================================================
.PHONY: provision
provision: ## Full provisioning (all layers + hardening)
	@mkdir -p $(LOG_DIR)
	$(PROVISION_SH) --target $(TOMB_TARGET)

.PHONY: provision-dry
provision-dry: ## Full provisioning dry run
	$(PROVISION_SH) --target $(TOMB_TARGET) --dry-run

.PHONY: setup-base
setup-base: ## Phase 1: Base system setup
	$(PROVISION_SH) --target $(TOMB_TARGET) --phase setup-base

.PHONY: deploy-lich
deploy-lich: ## Deploy Layer 2: Lich adversary framework
	$(PROVISION_SH) --target $(TOMB_TARGET) --phase deploy-lich

.PHONY: deploy-grimoire
deploy-grimoire: ## Deploy Layer 3: Grimoire knowledge base
	$(PROVISION_SH) --target $(TOMB_TARGET) --phase deploy-grimoire

.PHONY: deploy-oracle
deploy-oracle: ## Deploy Layer 4: Oracle LLM (Ollama + Mistral)
	$(PROVISION_SH) --target $(TOMB_TARGET) --phase deploy-oracle

.PHONY: deploy-dark-mirror
deploy-dark-mirror: ## Deploy Layer 5: Dark Mirror observability
	$(PROVISION_SH) --target $(TOMB_TARGET) --phase deploy-dark-mirror

.PHONY: harden
harden: ## Run Phase 13 security hardening
	$(PROVISION_SH) --target $(TOMB_TARGET) --phase harden

.PHONY: verify
verify: ## Run full deployment verification
	$(PROVISION_SH) --target $(TOMB_TARGET) --phase verify

# ============================================================================
# Ansible Playbooks
# ============================================================================
.PHONY: ansible-full
ansible-full: ## Run full Ansible deployment (all playbooks)
	cd $(ANSIBLE_DIR) && $(ANSIBLE_PLAY) playbook-full.yml

.PHONY: ansible-base
ansible-base: ## Run base system playbook
	cd $(ANSIBLE_DIR) && $(ANSIBLE_PLAY) playbook-base.yml

.PHONY: ansible-lich
ansible-lich: ## Run Lich deployment playbook
	cd $(ANSIBLE_DIR) && $(ANSIBLE_PLAY) playbook-lich.yml

.PHONY: ansible-grimoire
ansible-grimoire: ## Run Grimoire deployment playbook
	cd $(ANSIBLE_DIR) && $(ANSIBLE_PLAY) playbook-grimoire.yml

.PHONY: ansible-oracle
ansible-oracle: ## Run Oracle deployment playbook
	cd $(ANSIBLE_DIR) && $(ANSIBLE_PLAY) playbook-oracle.yml

.PHONY: ansible-dark-mirror
ansible-dark-mirror: ## Run Dark Mirror deployment playbook
	cd $(ANSIBLE_DIR) && $(ANSIBLE_PLAY) playbook-dark-mirror.yml

.PHONY: ansible-harden
ansible-harden: ## Run hardening playbook
	cd $(ANSIBLE_DIR) && $(ANSIBLE_PLAY) playbook-hardening.yml

.PHONY: ansible-check
ansible-check: ## Ansible dry-run (check mode, no changes)
	cd $(ANSIBLE_DIR) && $(ANSIBLE_PLAY) playbook-full.yml --check --diff

.PHONY: ansible-syntax
ansible-syntax: ## Syntax check all playbooks
	@echo "[INFO] Checking playbook syntax..."
	@for pb in $(ANSIBLE_DIR)/playbook-*.yml; do \
		echo "  Checking $$(basename $$pb)..."; \
		cd $(ANSIBLE_DIR) && ansible-playbook -i $(INVENTORY) $$pb --syntax-check || exit 1; \
	done
	@echo "[  OK] All playbooks pass syntax check"

# ============================================================================
# Testing
# ============================================================================
.PHONY: test
test: test-connectivity test-airgap test-layers test-services ## Run all integration tests

.PHONY: test-connectivity
test-connectivity: ## Test SSH and bridge connectivity
	@echo "[TEST] SSH connectivity..."
	@ssh $(SSH_OPTS) $(TOMB_USER)@$(TOMB_TARGET) "echo 'SSH OK'" 2>/dev/null \
		&& echo "[PASS] SSH to $(TOMB_TARGET)" \
		|| echo "[FAIL] SSH to $(TOMB_TARGET)"
	@echo "[TEST] Bridge connectivity..."
	@ssh $(SSH_OPTS) $(TOMB_USER)@$(TOMB_TARGET) "ping -c 1 -W 2 $(HOST_IP)" >/dev/null 2>&1 \
		&& echo "[PASS] Bridge $(TOMB_TARGET) -> $(HOST_IP)" \
		|| echo "[FAIL] Bridge $(TOMB_TARGET) -> $(HOST_IP)"

.PHONY: test-airgap
test-airgap: ## Verify air-gap enforcement
	@echo "[TEST] Air-gap enforcement..."
	@ssh $(SSH_OPTS) $(TOMB_USER)@$(TOMB_TARGET) "ping -c 1 -W 2 8.8.8.8" >/dev/null 2>&1 \
		&& echo "[FAIL] AIR-GAP VIOLATION: Can reach 8.8.8.8" \
		|| echo "[PASS] Air-gap intact (8.8.8.8 unreachable)"
	@ssh $(SSH_OPTS) $(TOMB_USER)@$(TOMB_TARGET) "ping -c 1 -W 2 1.1.1.1" >/dev/null 2>&1 \
		&& echo "[FAIL] AIR-GAP VIOLATION: Can reach 1.1.1.1" \
		|| echo "[PASS] Air-gap intact (1.1.1.1 unreachable)"

.PHONY: test-layers
test-layers: ## Verify all 5 layers are deployed
	@echo "[TEST] Layer verification..."
	@ssh $(SSH_OPTS) $(TOMB_USER)@$(TOMB_TARGET) "\
		pass=0; total=5; \
		test -d /opt/tomb/lich/campaigns/LICH-001 && pass=\$$((pass+1)) && echo '[PASS] Layer 2: Lich' || echo '[FAIL] Layer 2: Lich'; \
		test -d /opt/tomb/grimoire/kingdom-docs && pass=\$$((pass+1)) && echo '[PASS] Layer 3: Grimoire' || echo '[FAIL] Layer 3: Grimoire'; \
		test -f /opt/tomb/oracle/rag-config.json && pass=\$$((pass+1)) && echo '[PASS] Layer 4: Oracle' || echo '[FAIL] Layer 4: Oracle'; \
		test -f /opt/tomb/dark-mirror/prometheus/prometheus.yml && pass=\$$((pass+1)) && echo '[PASS] Layer 5: Dark Mirror' || echo '[FAIL] Layer 5: Dark Mirror'; \
		sudo iptables -L -n 2>/dev/null | grep -q DROP && pass=\$$((pass+1)) && echo '[PASS] Hardening: Firewall' || echo '[FAIL] Hardening: Firewall'; \
		echo \"\$$pass/\$$total layers verified\"" 2>/dev/null \
	|| echo "[FAIL] Could not verify layers (SSH failed)"

.PHONY: test-services
test-services: ## Test running services inside Tomb
	@echo "[TEST] Service health..."
	@ssh $(SSH_OPTS) $(TOMB_USER)@$(TOMB_TARGET) "\
		for svc in ollama tomb-prometheus tomb-grafana tomb-loki; do \
			if systemctl is-active \$$svc >/dev/null 2>&1; then \
				echo \"[PASS] \$$svc: active\"; \
			else \
				echo \"[FAIL] \$$svc: \$$(systemctl is-active \$$svc 2>/dev/null)\"; \
			fi; \
		done; \
		curl -sf http://127.0.0.1:11434/api/tags >/dev/null 2>&1 \
			&& echo '[PASS] Ollama API responding on :11434' \
			|| echo '[FAIL] Ollama API not responding'; \
		curl -sf http://127.0.0.1:9090/-/healthy >/dev/null 2>&1 \
			&& echo '[PASS] Prometheus healthy on :9090' \
			|| echo '[FAIL] Prometheus not responding'; \
		curl -sf http://127.0.0.1:3100/ready >/dev/null 2>&1 \
			&& echo '[PASS] Loki ready on :3100' \
			|| echo '[FAIL] Loki not responding'" 2>/dev/null \
	|| echo "[FAIL] Could not test services (SSH failed)"

# ============================================================================
# Utilities
# ============================================================================
.PHONY: ssh
ssh: ## SSH into the Tomb VM
	ssh $(SSH_OPTS) $(TOMB_USER)@$(TOMB_TARGET)

.PHONY: logs
logs: ## Tail the latest provision log
	@latest=$$(ls -1t $(LOG_DIR)/provision-*.log 2>/dev/null | head -1); \
	if [ -n "$$latest" ]; then \
		echo "[INFO] Tailing: $$latest"; \
		tail -f "$$latest"; \
	else \
		echo "[INFO] No provision logs found in $(LOG_DIR)"; \
	fi

.PHONY: clean
clean: ## Clean staging files and old logs
	@echo "[INFO] Cleaning staging and old logs..."
	@ssh $(SSH_OPTS) $(TOMB_USER)@$(TOMB_TARGET) "sudo rm -rf /tmp/tomb-staging" 2>/dev/null || true
	@find $(LOG_DIR) -name "provision-*.log" -mtime +30 -delete 2>/dev/null || true
	@echo "[  OK] Clean complete"

.PHONY: stage-packages
stage-packages: ## Show instructions for staging air-gapped packages
	@echo ""
	@echo "  Pre-Stage Packages for Air-Gapped Deployment"
	@echo "  ============================================="
	@echo ""
	@echo "  All packages must be downloaded on an internet-connected host"
	@echo "  and placed in $(PACKAGES_DIR)/"
	@echo ""
	@echo "  Directory structure:"
	@echo "    packages/"
	@echo "    +-- debs/                  Debian packages for Kali"
	@echo "    +-- lich/                  Lich harness binaries"
	@echo "    +-- grimoire/"
	@echo "    |   +-- kingdom-docs/      Kingdom documentation"
	@echo "    |   +-- mitre-attck/       MITRE ATT&CK JSON"
	@echo "    |   +-- nvd-cves/          NVD CVE records"
	@echo "    |   +-- cwe/               CWE mappings"
	@echo "    |   +-- rag-deps/          Python wheels (chromadb, sentence-transformers)"
	@echo "    +-- oracle/"
	@echo "    |   +-- ollama             Ollama binary"
	@echo "    |   +-- models/            Mistral-7B model files"
	@echo "    +-- dark-mirror/"
	@echo "        +-- prometheus         Prometheus binary"
	@echo "        +-- grafana-server     Grafana binary"
	@echo "        +-- loki               Loki binary"
	@echo "        +-- promtail           Promtail binary"
	@echo ""
	@echo "  Staging commands (run on internet-connected host):"
	@echo ""
	@echo "    # Ollama"
	@echo "    curl -fsSL https://ollama.com/install.sh | sh"
	@echo "    ollama pull mistral"
	@echo "    cp /usr/local/bin/ollama $(PACKAGES_DIR)/oracle/ollama"
	@echo "    cp -r ~/.ollama/models/ $(PACKAGES_DIR)/oracle/models/"
	@echo ""
	@echo "    # RAG dependencies"
	@echo "    pip download -d $(PACKAGES_DIR)/grimoire/rag-deps/ chromadb sentence-transformers"
	@echo ""
	@echo "    # MITRE ATT&CK"
	@echo "    git clone https://github.com/mitre/cti.git $(PACKAGES_DIR)/grimoire/mitre-attck/"
	@echo ""
	@echo "    # Prometheus/Grafana/Loki (download from release pages)"
	@echo ""
