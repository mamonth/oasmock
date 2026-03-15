## Why

The project currently has specification documents (CLI, OpenAPI extensions, management API) but lacks a working implementation. A Go-based mock server is needed to provide a high-performance, portable tool for API mocking that supports the defined OpenAPI extensions and runtime expressions. This implementation will enable teams to use the mock server for testing and development, replacing the existing Node.js-based solution.

## What Changes

- Implement a Go command-line tool (`oasmock`) that starts an HTTP mock server
- Parse OpenAPI 3.0 schemas with custom extensions (`x-mock-*`) described in [extensions.md](../../../extensions.md)
- Support runtime expressions and state management as described in [extensions.md](../../../extensions.md)
- Provide the management HTTP API (`/_mock/examples`, `/_mock/requests`) for dynamic control as described in [openapi.yaml](../../../openapi.yaml)
- Follow the CLI interface defined in cli.md (options, environment variables, return codes) as described in [cli.md](../../../cli.md)
- Include validation, CORS handling, request delay simulation, and verbose logging

## Capabilities

### New Capabilities
- `cli`: Command-line interface specification for starting and configuring the mock server. Maps to [cli.md](../../../cli.md).
- `extensions`: OpenAPI extensions support including mock-specific extensions (`x-mock-params-match`, `x-mock-skip`, `x-mock-set-state`, `x-mock-headers`, `x-mock-once`) with runtime expression evaluation.
- `management-api`: HTTP API for managing the mock server at runtime (adding examples, retrieving request history). Maps to [openapi.yaml](../../../openapi.yaml).
- `mock-server-core`: Core mock server functionality: loading OpenAPI schemas, routing requests, matching examples, applying extensions, generating responses, and maintaining state.

### Modified Capabilities
- None (no existing specs to modify)

## Impact

- New Go codebase in the repository
- Dependencies on Go libraries for OpenAPI parsing (`kin-openapi`), HTTP server, JSON Schema validation
- Cross-platform binary distribution (Linux, macOS, Windows, NPM package, Docker image)
- Integration with CI/CD pipelines for API testing
- Documentation updates to include installation and usage instructions
