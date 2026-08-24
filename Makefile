.PHONY: test test-v cover clean build lint test-debian test-all

# Default target
all: build

# Run all tests
test:
	go test ./...

# Run debian maintainer-script unit tests (no package install required)
test-debian:
	sh debian/tests/unit/run.sh

# Run Go + debian unit tests
test-all: test test-debian

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
	rm -f unitpm unitpmd
