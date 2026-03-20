# Makefile for oasmock

.PHONY: help build build-cross test test-unit test-integration lint clean coverage-unit spec-coverage

# Default target
all: build

# Help target
help:
	@echo "Available targets:"
	@echo "  build          - compile the binary"
	@echo "  build-cross    - cross-compile for linux, darwin, windows"
	@echo "  test           - run all tests (unit + integration)"
	@echo "  test-unit      - run unit tests only"
	@echo "  test-integration - run integration tests only"
	@echo "  lint           - run linter (golangci-lint)"
	@echo "  clean          - remove build artifacts"
	@echo "  install        - install dependencies (go mod tidy)"
	@echo "  generate       - run go generate"
	@echo "  coverage-unit  - run test coverage check for unit tests only"
	@echo "  spec-coverage  - check requirement scenario coverage"

# Install dependencies
install:
	go mod tidy

# Build binary
build:
	go build -o bin/oasmock ./cmd/oasmock

# Cross-compile for all platforms
build-cross:
	GOOS=linux GOARCH=amd64 go build -o bin/oasmock-linux-amd64 ./cmd/oasmock
	GOOS=darwin GOARCH=amd64 go build -o bin/oasmock-darwin-amd64 ./cmd/oasmock
	GOOS=windows GOARCH=amd64 go build -o bin/oasmock-windows-amd64.exe ./cmd/oasmock

# Run tests
test:
	go test ./...

# Run unit tests only
test-unit:
	go test $(shell go list ./... | grep -v /test)

# Run integration tests only
test-integration:
	go test ./test/...

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
