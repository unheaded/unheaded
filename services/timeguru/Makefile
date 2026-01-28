.PHONY: test test-race test-coverage bench lint build clean run

# Service name
NAME := timeguru

# Build output
BIN := bin/$(NAME)

# Default target
all: test build

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with race detector
test-race:
	@echo "Running tests with race detector..."
	go test -v -race ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@echo ""
	@echo "Coverage by package:"
	@go tool cover -func=coverage.out | grep -v "100.0%" || true
	@echo ""
	@echo "Total coverage:"
	@go tool cover -func=coverage.out | grep total

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem -run=^$ ./...

# Lint code
lint:
	@echo "Linting..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed" && exit 1)
	golangci-lint run ./...

# Build binary
build:
	@echo "Building $(NAME)..."
	@mkdir -p bin
	go build -o $(BIN) ./cmd/$(NAME)
	@echo "Built: $(BIN)"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/ coverage.out

# Run service locally
run: build
	@echo "Running $(NAME)..."
	./$(BIN)

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Development mode with auto-restart
dev:
	@echo "Starting development mode..."
	@which air > /dev/null || (echo "Install air: go install github.com/cosmtrek/air@latest" && exit 1)
	air
