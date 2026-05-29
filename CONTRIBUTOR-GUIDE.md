# Contributor Guide: The Unheaded Kingdom

Welcome to the Unheaded Kingdom protocol project. This guide outlines how to contribute to the protocol, implementation, and documentation.

## 1. Welcome

The Unheaded Kingdom is in **alpha** — your contributions directly shape the protocol and the future of the Kenoma. Contributors are initiates of the Kenoma, exploring the depths of distributed tracing and eBPF-based observability at wire speed.

We welcome contributions from developers, protocol designers, security researchers, and operators. Whether you're improving the core protocol, enhancing the eBPF implementation, or refining the documentation, you are building something that matters.

**Contact & Community:**
- Technical questions & issue reports: [GitHub Issues](https://github.com/stevebellis/unheaded)
- Direct contact: stevie@bellis.tech
- Protocol working group: TBD (will be created at bellis.tech/unheaded/wg)

---

## 2. Developer Certificate of Origin (DCO)

Unheaded is GPL-3.0 licensed. No CLA required. We use the **Developer Certificate of Origin (DCO)** — a lightweight sign-off confirming you have the right to contribute your code.

### What the DCO Says

By adding a `Signed-off-by` line to your commits, you certify:

1. The contribution was created in whole or in part by you and you have the right to submit it under the GPL-3.0 License.
2. The contribution is based upon previous work that, to the best of your knowledge, is covered under an appropriate open source license and you have the right to submit that work with modifications under the GPL-3.0 License.
3. The contribution was provided directly to you by some other person who certified (1) or (2) and you have not modified it.

Full DCO text: https://developercertificate.org

### How to Sign Off

Add `-s` to your commit command:

```bash
git commit -s -m "feat: your change description"
```

This appends `Signed-off-by: Your Name <your@email.com>` to the commit message. That's it. No forms, no IP assignment, no legal overhead.

---

## 3. Development Setup

### Automated Setup (Recommended)

For a fresh Ubuntu 25.x machine, the bootstrap script installs **everything** in one shot:

```bash
sudo ./scripts/bootstrap-llm-lab.sh
```

This handles all prerequisites below plus GPU drivers, kernel tuning, eBPF environment, monitoring stack, secrets management, and LLM inference tooling. See the script for full details (16 phases).

### Prerequisites

| Tool | Version | Install | Check |
|------|---------|---------|-------|
| **Go** | 1.25+ | `wget https://go.dev/dl/go1.25.0.linux-amd64.tar.gz && tar -C /usr/local -xzf go*.tar.gz` | `go version` |
| **Rust** | nightly | `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \| sh -s -- -y --default-toolchain nightly` | `rustc --version` |
| **Rust eBPF targets** | — | `rustup target add x86_64-unknown-linux-musl && rustup component add rust-src && cargo install bpf-linker` | `cargo install --list \| grep bpf` |
| **Docker + Compose** | 24+ | [docs.docker.com/engine/install](https://docs.docker.com/engine/install/) | `docker compose version` |
| **LXD/Incus** | latest | `snap install lxd` | `lxd --version` |
| **Nix** | 2.18+ | `curl -L https://nixos.org/nix/install \| sh -s -- --daemon` | `nix --version` |
| **Node.js** | 22 LTS | `curl -fsSL https://deb.nodesource.com/setup_22.x \| bash && apt install nodejs` | `node --version` |
| **Claude Code** | latest | `npm install -g @anthropic-ai/claude-code` | `claude --version` |
| **Linux kernel** | 6.8+ | Ubuntu 25.x ships 6.14 | `uname -r` |
| **eBPF tools** | — | `apt install bpftool bpftrace bpfcc-tools clang llvm libbpf-dev` | `bpftool version` |
| **SOPS + age** | — | `apt install age` + [github.com/getsops/sops](https://github.com/getsops/sops/releases) | `sops --version && age --version` |

### Build Commands

**Go components:**
```bash
go build ./...
go test -race ./...
```

**Rust/eBPF components:**
```bash
cd crates/
cargo build --release
cargo test
```

**Full integration build:**
```bash
make build          # build all Go services
make ebpf           # build eBPF programs (Rust/Aya)
make containers     # build container images
make test           # run test suite
make dev            # local development (docker-compose)
```

For detailed setup and smoke tests, see [QUICKSTART.md](./QUICKSTART.md).

---

## 4. Code Style & Quality

### Go Code Style

- **Format**: Use `gofmt` (enforced automatically)
- **Linting**: All code must pass `golangci-lint` (config: `.golangci.yml`)
- **Testing**: 100% coverage required for core packages:
  - `pkg/auth`
  - `pkg/ports`
  - `pkg/transport`
  - `pkg/metrics`
  - `pkg/protocol`
- **Race Detection**: All tests run with `-race` flag enabled in CI

### Rust Code Style

- **Format**: Use `rustfmt`
- **Linting**: All code must pass `clippy`
- **Testing**: Full test coverage for new code
- **Security**: Review for unsafe code; all unsafe blocks must have `// SAFETY:` comments

### Assumptions About Input

**All external input is assumed hostile.** This includes:
- Network packets
- Configuration files
- User input from APIs
- Data from untrusted sources

Apply defensive programming practices (input validation, bounds checking, etc.) consistently.

---

## 5. Branch Strategy

### Branch Naming

Use the format: `sNN-description` where `NN` is the sprint number.

Examples:
- `s52-contributor-guide`
- `s51-security-hardening`
- `s50-protocol-optimization`

### Workflow

1. Create a branch from `main` with the sprint naming convention
2. Make commits with clear, atomic changes
3. Push to origin and create a pull request
4. Address code review feedback
5. Squash and merge into `main` (CI must pass)
6. Update battle plan in `docs/sessions/` with completion status

**No direct commits to `main` are permitted.**

---

## 6. The Kingdom Aesthetic

Code in the Unheaded Kingdom carries cultural weight. Contributors are encouraged to:

- Use Kingdom terminology in code comments and naming when appropriate (but *never* at the expense of code clarity)
- Service names follow **Gnostic/Medieval Armory naming conventions** (e.g., `moat-ghost`, `black-mage`, `paladin-bridge`)
- Naming conventions and lore: See `docs/lore/NAMING-CONVENTIONS.md`

Remember: **Clarity always trumps aesthetics.** If a Kingdom-themed name is confusing, use a descriptive name instead.

---

## 7. Protocol Changes

Any changes to the **Monad**, **Sophia**, or **Wotan** wire formats or protocol specifications require:

1. **RFC Editor Review**: Coordinate with the protocol working group
2. **IETF Internet-Draft**: Create or update a relevant `draft-bellis-unheaded-*` document
3. **IANA Considerations**: Address IANA registration for any new option types

For IPv6 extension header option types (e.g., Type 0x2A, 0x2B), see **docs/legal/IANA-REGISTRATION.md** for the registration strategy.

---

## 8. Security

### Reporting Security Issues

**Do not open public GitHub issues for security vulnerabilities.**

Email security reports to: **stevie@bellis.tech**

Include:
- Description of the vulnerability
- Steps to reproduce (if possible)
- Affected versions
- Your suggested fix (optional)

We will acknowledge receipt within 48 hours and provide updates on our investigation.

### Security Review Process

All PRs are automatically scanned by:
- **Moat Ghost**: Automated static analysis for authentication/authorization issues
- **Black Mage**: Review process for cryptographic and security-critical code

See [SECURITY.md](./SECURITY.md) for full disclosure policy and current security contacts.

---

## 9. Testing Requirements

### Unit Tests

- **Required** for all new public functions and exported APIs
- Use `go test` for Go code; `cargo test` for Rust
- Tests must pass with race detector enabled: `go test -race ./...`

### Integration Tests

- **Required** for cross-service features
- Test protocol interactions (Monad ↔ Sophia, etc.)
- Located in `tests/integration/`

### Fuzz Testing

- **Required** for parser and serializer code
- Use LICH framework: See `crates/lich/` for fuzz targets
- Run with: `cargo fuzz` (in `crates/`)

### CI Requirements

All checks must pass before merge:
```
go fmt ./...        # Format check
golangci-lint       # Linting
go test -race ./... # Unit tests with race detector
cargo test          # Rust tests
cargo fuzz          # Fuzz targets
docker build        # Container build
```

---

## 10. Documentation

### Required Documentation

- **Code comments**: Export all public types and functions with godoc comments
- **Commit messages**: Clear, descriptive messages explaining the "why"
- **Battle plans**: Update `docs/sessions/` with sprint progress
- **Protocol docs**: Update `docs/protocol/` for wire format changes
- **Architecture**: Update `docs/architecture/` for structural changes

### Documentation Format

- Markdown for guides and protocols
- Inline comments for complex logic
- Architecture Decision Records (ADRs) in `docs/adr/`

---

## 11. Commit Message Format

Commit messages should follow this structure:

```
[s52] Brief description of the change

More detailed explanation of what changed and why. Explain the problem
being solved and the approach taken.

If this closes an issue, include:
Fixes #123
```

Examples:
```
[s52] Add CONTRIBUTOR-GUIDE.md and IANA registration strategy

Documents the CLA terms and development workflow for contributors,
and outlines the path to IANA registration for IPv6 extension
header option types.
```

---

## 12. Pull Request Process

1. **Create a PR** with a clear title and description
2. **Link issues**: Reference related issues (e.g., `Fixes #123`)
3. **Add testing evidence**: Describe how you tested the change
4. **Sign-off**: Commits must have `Signed-off-by` (use `git commit -s`)
5. **Address feedback**: Respond to code review comments
6. **Ensure CI passes**: All automated checks must pass
7. **Merge**: Once approved, maintainers will merge

---

## 13. Getting Help

**Stuck?** Here's where to get help:

- **Go questions**: See `docs/ARCHITECTURE.md` and code examples in `pkg/`
- **eBPF questions**: See `ebpf/` and `crates/` directories
- **Protocol questions**: See `docs/protocol/` and `references/`
- **Build issues**: Check [QUICKSTART.md](./QUICKSTART.md)
- **GitHub discussions**: Open a discussion for non-urgent questions
- **Email**: stevie@bellis.tech for protocol or strategic questions

---

## 14. Code of Conduct

We expect all contributors to:

- Be respectful and constructive
- Assume good faith in others' intentions
- Provide helpful feedback
- Respect confidentiality of security reports
- Follow the law and ethical guidelines

Violations may result in removal from the project.

---

## References

- [SECURITY.md](./SECURITY.md) — Security policy and disclosure
- [docs/legal/IANA-REGISTRATION.md](./docs/legal/IANA-REGISTRATION.md) — IANA registration strategy
- [QUICKSTART.md](./QUICKSTART.md) — Development setup
- [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md) — Architecture overview
- [docs/protocol/](./docs/protocol/) — Protocol specifications

---

**Last updated**: 2026-02-25  
**Sprint**: S52 Legal Sprint  
**Contact**: stevie@bellis.tech
