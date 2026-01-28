.PHONY: help test test-race test-coverage test-verbose lint build clean bench

# Variables
SERVICE_NAME = micromanager
BINARY_PATH = ../../bin/$(SERVICE_NAME)
CMD_PATH = ./cmd
MAIN_GO = $(CMD_PATH)/main.go

help:
	@echo "Micromanager Service - Build & Test Targets"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  test              Run unit tests"
	@echo "  test-race         Run tests with race detection"
	@echo "  test-coverage     Run tests with coverage report"
	@echo "  test-verbose      Run tests with verbose output"
	@echo "  lint              Run linter (golangci-lint)"
	@echo "  build             Build binary"
	@echo "  clean             Remove build artifacts"
	@echo "  bench             Run benchmarks"
	@echo "  help              Show this help message"

# Run unit tests
test:
	@echo "Running unit tests..."
	@cd ../.. && go test -v ./services/micromanager/...

# Run tests with race detector
test-race:
	@echo "Running tests with race detection..."
	@cd ../.. && go test -v -race ./services/micromanager/...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@cd ../.. && go test -coverprofile=services/micromanager/coverage.out ./services/micromanager/...
	@cd ../.. && go tool cover -func=services/micromanager/coverage.out
	@echo ""
	@echo "Full report: go tool cover -html=services/micromanager/coverage.out"

# Run tests with verbose output
test-verbose:
	@echo "Running tests with verbose output..."
	@cd ../.. && go test -v -race -cover ./services/micromanager/...

# Lint code
lint:
	@echo "Linting code..."
	@cd ../.. && golangci-lint run ./services/micromanager/...

# Build binary
build:
	@echo "Building $(SERVICE_NAME) binary..."
	@mkdir -p ../../bin
	@cd ../.. && go build -o $(BINARY_PATH) $(CMD_PATH)/main.go
	@echo "Built: $(BINARY_PATH)"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -f $(BINARY_PATH)
	@rm -f services/micromanager/coverage.out
	@echo "Clean complete"

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	@cd ../.. && go test -bench=. -benchmem -run=^$ ./services/micromanager/...

# Run all tests with summary
all: clean test-race test-coverage
	@echo ""
	@echo "✅ All tests passed!"
	@echo ""
	@echo "Summary:"
	@echo "  - Unit tests with race detection: PASSED"
	@echo "  - Coverage report generated"
	@echo ""
	@echo "Next steps:"
	@echo "  1. Review coverage: go tool cover -html=services/micromanager/coverage.out"
	@echo "  2. Build binary: make build"
	@echo "  3. Start service: ./bin/micromanager"
