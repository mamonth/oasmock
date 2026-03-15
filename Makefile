# Makefile for oasmock

.PHONY: help build test test-unit test-integration lint clean coverage coverage-unit spec-coverage

# Default target
all: build

# Help target
help:
	@echo "Available targets:"
	@echo "  build          - compile the binary"
	@echo "  test           - run all tests (unit + integration)"
	@echo "  test-unit      - run unit tests only"
	@echo "  test-integration - run integration tests only"
	@echo "  lint           - run linter (golangci-lint)"
	@echo "  clean          - remove build artifacts"
	@echo "  install        - install dependencies (go mod tidy)"
	@echo "  generate       - run go generate"
	@echo "  coverage       - run test coverage check (all tests)"
	@echo "  coverage-unit  - run test coverage check for unit tests only"
	@echo "  spec-coverage  - check requirement scenario coverage"

# Install dependencies
install:
	go mod tidy

# Build binary
build:
	go build -o bin/oasmock ./cmd/oasmock

# Run tests
test:
	go test ./...

# Run unit tests only
test-unit:
	go test $(shell go list ./... | grep -v /test)

# Run integration tests only
test-integration: build
	go test ./test/...

# Run tests with coverage check (70% threshold - current baseline, must not regress)
coverage:
	./scripts/check-coverage.sh 70

# Run coverage check for unit tests only
coverage-unit:
	./scripts/check-coverage.sh 70 "$(shell go list ./... | grep -v /mock | grep -v /test | tr '\n' ' ')"

# Check requirement scenario coverage
spec-coverage:
	python3 scripts/analyze_scenario_coverage.py --detailed --output coverage_report.md

# Run linter (requires golangci-lint)
lint:
	golangci-lint run

# Clean up
clean:
	rm -rf bin/

# Generate code
generate:
	go generate ./...

# Install golangci-lint (if not present)
install-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin v1.61.0