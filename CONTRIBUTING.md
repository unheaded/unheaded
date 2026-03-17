# Contributing to Unheaded

Thank you for your interest in contributing to Unheaded! We welcome contributions from the community.

## Code of Conduct

See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for our community standards.

## Development Setup

Prerequisites:
- Linux kernel 5.15+
- Go 1.24+
- Docker or LXD
- Rust 1.70+ (for eBPF)

Clone and setup:
```bash
git clone https://github.com/unheaded/unheaded.git
cd unheaded
go mod download
go build ./...
```

## Build & Test

```bash
# Build all services
go build ./...

# Run tests
go test ./... -short

# Build eBPF programs (requires Rust + Aya)
cd ebpf && cargo build --release
```

## Commit Guidelines

Use Conventional Commits format:

```
<type>(<scope>): <subject>

[optional body]
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation
- `test`: Tests
- `chore`: Build, dependencies
- `refactor`: Code restructuring

**Example:**
```
feat(wotan): add gRPC reflection support

Enables dynamic service discovery via gRPC reflection API.
```

## Pull Request Process

1. Fork the repository
2. Create feature branch: `git checkout -b feature/your-feature`
3. Make changes and test locally
4. Commit with Conventional Commits
5. Push to your fork
6. Open a pull request against `main`
7. Address review feedback
8. Maintainer merges when approved

## Testing Requirements

- Unit test coverage: minimum 80%
- All tests must pass: `go test ./...`
- No skipped tests without justification
- Integration tests must pass on branch

## Code Style

- Follow [Effective Go](https://go.dev/doc/effective_go)
- Use `goimports` for imports
- Keep lines under 100 characters where practical

## Security

- No hardcoded secrets (use environment variables or SOPS)
- All external dependencies must be reviewed
- Security fixes: email stevie@bellis.tech before public disclosure

## Questions?

- GitHub Issues: For bugs and feature requests
- Discussions: For architecture and design questions
- Email: stevie@bellis.tech

---

**Last Updated:** March 17, 2026
