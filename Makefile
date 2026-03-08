.PHONY: test test-v cover clean build lint

# Default target
all: build

# Run all tests
test:
	go test ./...

# Run all tests with verbose output
test-v:
	go test -v ./...

# Run tests with coverage
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

# Build the project
build:
	go build ./...

# Run linter
lint:
	golangci-lint run

# Clean build artifacts
clean:
	rm -f coverage.out
	rm -f lynx lynxd
