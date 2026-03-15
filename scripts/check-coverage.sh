#!/usr/bin/env bash
set -euo pipefail

# Check coverage threshold (default 70% - current baseline, must not regress)
THRESHOLD=${1:-70}

# Default package pattern excludes mock/ (generated code) and test/ (integration tests)
if [ -z "${2:-}" ]; then
    PACKAGE_PATTERN=$(go list ./... | grep -v /mock | grep -v /test)
else
    PACKAGE_PATTERN=${2}
fi

echo "Running test coverage analysis with threshold ${THRESHOLD}%..."
echo "Package pattern: ${PACKAGE_PATTERN}"

# Run tests with coverage
go test -coverprofile=coverage.out ${PACKAGE_PATTERN}

# Generate coverage report
COVERAGE_REPORT=$(go tool cover -func=coverage.out)

# Extract total coverage percentage
TOTAL_COVERAGE=$(echo "${COVERAGE_REPORT}" | grep -E '^total:\s+\(statements\)' | awk '{print $3}' | sed 's/%//')

echo "Coverage report:"
echo "${COVERAGE_REPORT}"
echo ""
echo "Total coverage: ${TOTAL_COVERAGE}% (threshold: ${THRESHOLD}%)"

# Compare with threshold
if (( $(echo "${TOTAL_COVERAGE} < ${THRESHOLD}" | bc -l) )); then
    echo "ERROR: Coverage ${TOTAL_COVERAGE}% is below required ${THRESHOLD}%"
    exit 1
else
    echo "SUCCESS: Coverage meets threshold"
fi

# Clean up
rm -f coverage.out