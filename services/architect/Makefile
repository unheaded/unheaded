.PHONY: test test-race test-coverage lint build clean bench

# Test targets
test:
	go test -v ./...

test-race:
	go test -v -race ./...

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@echo ""
	@echo "HTML coverage report: open coverage.html"
	go tool cover -html=coverage.out -o coverage.html

test-coverage-threshold:
	go test -coverprofile=coverage.out ./...
	@threshold=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	echo "Coverage: $$threshold%"; \
	if [ "$$(echo "$$threshold < 80" | bc)" -eq 1 ]; then \
		echo "ERROR: Coverage below 80%"; \
		exit 1; \
	fi

# Lint
lint:
	go fmt ./...
	go vet ./...

# Build
build:
	go build -o bin/architect ./cmd

build-linux:
	GOOS=linux GOARCH=amd64 go build -o bin/architect-linux-amd64 ./cmd

# Benchmarks
bench:
	go test -bench=. -benchmem -run=^$ ./...

# Clean
clean:
	rm -rf bin/ coverage.out coverage.html

# Development
dev:
	go run ./cmd -addr :19001 -wotan localhost:18001 -log debug -mock-wotan

# Docker
docker-build:
	docker build -t unheaded/architect:latest .

# Installation
install: build
	cp bin/architect $(GOBIN)/architect

# All tests
test-all: test-race test-coverage-threshold lint
	@echo "✓ All tests passed"
