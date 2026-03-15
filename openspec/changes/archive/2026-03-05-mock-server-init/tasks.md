## 1. Project Setup

- [x] 1.1 Initialize Go module (`go mod init github.com/.../oasmock`)
- [x] 1.2 Create basic directory structure (cmd/, internal/, pkg/)
- [x] 1.3 Add dependencies: cobra, kin-openapi, chi, gojsonschema (or alternative), slog
- [x] 1.4 Set up Makefile or scripts for build, test, lint
- [x] 1.5 Configure CI/CD (GitHub Actions) for Go builds and tests

## 2. CLI Implementation

- [x] 2.1 Implement root command with cobra, add --version and --help flags
- [x] 2.2 Implement mock command with flag definitions (--from, --prefix, --port, --delay, --verbose, --nocors)
- [x] 2.3 Bind environment variables (OASMOCK_PORT, OASMOCK_VERBOSE, OASMOCK_NO_CORS)
- [x] 2.4 Implement CLI validation and exit codes as per spec
- [x] 2.5 Add integration tests for CLI flags and environment overrides

## 3. OpenAPI Schema Loading

- [x] 3.1 Implement schema loader that reads one or more OpenAPI YAML/JSON files
- [x] 3.2 Support path prefixes for each schema
- [x] 3.3 Validate schemas using kin-openapi, exit with code 3 on failure
- [x] 3.4 Merge multiple schemas into a unified router mapping (preserving prefixes)

## 4. Runtime Expression Engine

- [x] 4.1 Design data source providers (request, state, env)
- [x] 4.2 Implement expression parser for syntax `{$request.path.param}`
- [x] 4.3 Implement dot‑notation navigation with escaping for dots in property names
- [x] 4.4 Implement modifiers: default, getByPath, toJWT (stub for now)
- [x] 4.5 Add unit tests for expression evaluation with various data sources

## 5. Mock Server Core

- [x] 5.1 Implement HTTP server with chi router, CORS middleware, request delay middleware
- [x] 5.2 Implement request routing: match incoming request to OpenAPI operation
- [x] 5.3 Implement example selection algorithm (skip, params-match, once)
- [x] 5.4 Implement JSON Schema validation for x-mock-params-match conditions
- [x] 5.5 Implement state management per schema namespace (get/set/increment/delete)
- [x] 5.6 Implement response generation: status code, headers, body
- [x] 5.7 Implement request history recording (in‑memory ring buffer)
- [x] 5.8 Add verbose logging middleware (log request/response details)

## 6. Mock Extensions

- [x] 6.1 Implement x-mock-params-match evaluation (basic, needs debugging)
- [x] 6.2 Implement x-mock-skip filtering
- [x] 6.3 Implement x-mock-set-state for setting, incrementing, deleting state
- [x] 6.4 Implement x-mock-headers for setting response headers
- [x] 6.5 Implement x-mock-once (one‑time example removal)

## 7. Management API

- [x] 7.1 Implement POST /_mock/examples handler for adding dynamic examples
- [x] 7.2 Implement GET /_mock/requests handler with filtering (path, method, time range, pagination)
- [x] 7.3 Validate request bodies against OpenAPI schemas (AddExampleRequest)
- [x] 7.4 Integrate management API routes into the main server

## 8. Integration and Testing

- [x] 8.1 End‑to‑end test: start server with sample OpenAPI schema, send requests, verify responses
- [x] 8.2 Test mock extensions: params‑match, set‑state, headers, once
- [x] 8.3 Test management API: add example, retrieve request history
- [x] 8.4 Test CLI options and environment variables
- [x] 8.5 Performance test: measure overhead of JSON schema validation and expression evaluation

## 9. Documentation and Packaging

- [x] 9.1 Write README with installation and usage instructions
- [x] 9.2 Document extension syntax and runtime expression examples (partially)
- [x] 9.3 Create example OpenAPI schemas demonstrating all features
- [x] 9.4 Create github action pipelines for test, build and release cross‑platform binaries (Linux, macOS, Windows, npm) via goreleaser
