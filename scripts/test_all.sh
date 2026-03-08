#!/bin/bash
set -e

echo "Running all tests..."
go test -v ./...

echo "Tests passed!"
