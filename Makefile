# Makefile for oasmock

.PHONY: help build build-cross test test-unit test-integration lint clean coverage-unit spec-coverage docker-build

# Default target
all: build

# Help target
help:
	@echo "Available targets:"
	@echo "  build          - compile the binary"
	@echo "  build-cross    - cross-compile for linux (amd64/arm64), darwin, windows"
	@echo "  test           - run all tests (unit + integration)"
	@echo "  test-unit      - run unit tests only"
	@echo "  test-integration - run integration tests only"
	@echo "  lint           - run linter (golangci-lint)"
	@echo "  clean          - remove build artifacts"
	@echo "  install        - install dependencies (go mod tidy)"
	@echo "  generate       - run go generate"
	@echo "  coverage-unit  - run test coverage check for unit tests only"
	@echo "  spec-coverage  - check requirement scenario coverage"
	@echo "  docker-build   - build Docker image from local binary"

# Install dependencies
install:
	go mod tidy

# Build binary
build:
	go build -o bin/oasmock ./cmd/oasmock

# Cross-compile for all platforms
build-cross:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/oasmock-linux-amd64 ./cmd/oasmock
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/oasmock-linux-arm64 ./cmd/oasmock
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o bin/oasmock-darwin-amd64 ./cmd/oasmock
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/oasmock-windows-amd64.exe ./cmd/oasmock

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

# Build Docker image from locally compiled binary
docker-build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o oasmock-linux-amd64 ./cmd/oasmock
	docker build -t oasmock:dev .
	rm -f oasmock-linux-amd64

# Generate code
generate:
	go generate ./...

# Install golangci-lint (if not present)
install-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin v1.61.0
