.PHONY: test test-race test-coverage bench build run clean help

# Service name
NAME := dashboard-backend

# Build output
BIN := bin/$(NAME)

# Default target
.DEFAULT_GOAL := help

## help: Show this help message
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

## test: Run unit tests
test:
	go test -v ./...

## test-race: Run tests with race detector
test-race:
	go test -v -race ./...

## test-coverage: Run tests with coverage report
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@echo ""
	@echo "Coverage summary:"
	@go tool cover -func=coverage.out | grep -E 'total:|^github.com/unheaded/unheaded/cmd/dashboard-backend/internal' | grep -v 'test.go'

## test-coverage-html: Generate HTML coverage report
test-coverage-html: test-coverage
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## bench: Run benchmarks
bench:
	go test -bench=. -benchmem -run=^$$ ./...

## build: Build the dashboard-backend binary
build:
	@echo "Building $(NAME)..."
	@mkdir -p bin
	go build -o $(BIN) .
	@echo "Built: $(BIN)"

## run: Build and run the dashboard-backend
run: build
	./$(BIN) -debug

## run-wotan: Run with custom Wotan address
run-wotan: build
	./$(BIN) -wotan $(WOTAN_ADDR) -debug

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## fmt: Format Go code
fmt:
	go fmt ./...
	gofmt -s -w .

## vet: Run go vet
vet:
	go vet ./...

## clean: Remove build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

## deps: Download dependencies
deps:
	go mod download
	go mod tidy

## verify: Run all verification checks
verify: fmt vet lint test-race

## security: Run security audit
security:
	@echo "Running security audit..."
	@which gosec > /dev/null || (echo "Installing gosec..." && go install github.com/securego/gosec/v2/cmd/gosec@latest)
	gosec -exclude=G104 ./...

## docker-build: Build Docker image (placeholder)
docker-build:
	@echo "Docker build not yet implemented"

## docker-run: Run in Docker (placeholder)
docker-run:
	@echo "Docker run not yet implemented"

## install: Install the binary to GOPATH/bin
install:
	go install .

## all: Run fmt, vet, test-race, and build
all: fmt vet test-race build
	@echo ""
	@echo "✅ All checks passed!"
