# Project Context

## Tech Stack
- Golang (latest features, use `use-modern-go` skill after editing)
- gomock for generating mocks
- slog for logging
- Testify for test assertions

## Project structure
- `openspec/` - [OpenSpec](https://github.com/Fission-AI/OpenSpec) based project specifications
- `api/` - Management HTTP API docs (e.g. OpenAPI specs)
- `cmd/` - CLI entrypoints codebase
- `internal/` - Application codebase
- `mock/` - Mocks generated from interfaces, duplicates codebase structure
- `scripts/` - Various automation scripts
- `test/` - Integration tests and test related codebase and resources
  - `test/_shared` - Common files for tests codebase including fixtures, helper functions, resources etc
    - `test/_shared/resources` - Various resources (e.g. yaml, json files)
- `docs/` - Project documentation
  - `docs/diagrams` - PlantUML diagrams

## Conventions

### Mock generation
- Generate mocks into separate `mock/` directory with package suffix according to interface origin (e.g., `mock_runtime`)
- Use `_mock.go` suffix (not `_test.go`) for importability
- Exclude `mock/` directory from coverage reports and checks

## Testing standarts
- Use Testify package for all test assertions
- Define multiple test scenarios using slice of structs
- Each test should be marked with list of requirement scenario codes from [openspec's specs](`openspec/specs`)
- Unit tests: place near tested module with `_test.go` suffix
- Integration tests: place under `test/` directory, skip when `testing.Short()`

## Coverage Policy
- **Code coverage**: Minimum threshold (currently 70%) that must not regress
  - Excludes `mock/` directory (generated code) and `test/` directory (integration tests)
  - Threshold updated manually via PR when intentional coverage improvements are made
- **Requirement scenario coverage**: Must remain at 100%
  - All OpenSpec scenarios must have corresponding tests
  - Coverage report generated as CI artifact

