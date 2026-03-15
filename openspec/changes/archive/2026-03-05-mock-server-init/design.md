## Context

The OpenAPI Mock tool currently exists as a set of specification documents (CLI, OpenAPI extensions, management API) but lacks an implementation. A high‑performance, portable mock server written in Go is needed to replace the existing Node.js‑based `openapi‑toolkit mock` command. The server must support custom OpenAPI extensions for conditional examples, runtime expressions, state management, and dynamic control via an HTTP API.

## Goals / Non-Goals

**Goals:**
- Provide a single‑binary CLI tool (`oasmock`) that starts an HTTP mock server
- Load one or more OpenAPI 3.0 or OpenAPI 3.1 schemas
- Support mock‑specific extensions (`x-mock-match`, `x-mock-skip`, `x-mock-set-state`, `x-mock-headers`, `x-mock-once`)
- Evaluate runtime expressions with modifiers (`default`, `getByPath`, `toJWT`, etc.)
- Maintain isolated state per schema, updatable via extensions
- Expose a management HTTP API (`/_mock/examples`, `/_mock/requests`) for runtime control
- Follow the CLI specification (options, environment variables, exit codes)
- Include configurable request‑response delay, verbose logging, and CORS support
- Record request history for retrieval via the management API
- Validate examples against OpenAPI schemas when requested

**Non-Goals:**
- Graphical user interface or dashboard
- Authentication/authorization for the mock server (management API is unprotected)
- Persistent storage of state or history beyond server lifetime
- Support for OpenAPI 2.0 (Swagger)
- Real‑time collaboration or multi‑user features
- Load balancing or clustering across multiple instances

## Decisions

1. **OpenAPI parsing library**: Use `kin-openapi` – the most mature and actively maintained Go library for OpenAPI 3.0. It provides schema loading, validation, and path matching.

2. **HTTP router**: Use `chi` – a lightweight, idiomatic router that integrates well with `net/http`. It supports path parameters and middleware, making it suitable for prefix‑based multi‑schema routing.

3. **JSON Schema validation**: Use `github.com/xeipuuv/gojsonschema` or `github.com/santhosh-tekuri/jsonschema` for validating request parameters against schemas in `x-mock-match`. Need to evaluate performance and ease of use.

4. **Runtime expression engine**: Implement a custom interpreter that evaluates expressions like `{$request.path.id}`. Use a map of evaluators for different data sources (request, state, env). Modifiers will be implemented as chainable functions.

5. **State storage**: In‑memory map per schema namespace with mutex protection for concurrent access. State values can be any JSON‑serializable type.

6. **Example selection algorithm**: Iterate over operation’s examples in order, skip if `x-mock-skip`, 
   evaluate `x-mock-match` conditions (JSON schema validation), select first matching example. 
   If none match, fall back to first example without conditions.
   If no first example - return empty 404 response.

7. Evaluate mock extensions per request using the runtime expression engine.

8. **Management API implementation**: Same HTTP server as the mock endpoints, mounted under `/_mock`. Use separate handlers for `/_mock/examples` (POST) and `/_mock/requests` (GET).

9. **CLI framework**: Use `cobra` – provides subcommands, flag parsing, environment variable binding, and help generation that matches the CLI spec.

10. **Logging**: Use `slog` (standard library structured logging) with configurable verbosity.

11. **QA**: All use cases should be covered with unit and integration tests, without intersecting one another

12. **CI**: Github pipelines should be implemented, to follow the process: 
    - base (in parallel):
      - compile go binary -> docker image build
      - unit tests
    - integration tests: clean binary tests && docker image cases 
    - publication (in parallel):
      - new version binaries publication (win, mac, linux(appimage and binary)) using goReleaser
      - docker image, based on base docker hardened image (DHI)
      - npm package (using go-npm)

**Alternatives considered:**
- `gorilla/mux` instead of `chi` – both are fine; `chi` has slightly simpler API and better middleware support.
- `go‑openapi/runtime` – part of the go‑openapi toolkit, but heavier and oriented toward generating servers.
- Embedding a JavaScript engine for runtime expressions – too heavy; custom interpreter is more appropriate for the limited expression language.
- Persistent state (e.g., boltdb) – adds complexity; in‑memory state suffices for mocking use cases.

## Risks / Trade-offs

**Performance vs. correctness**: Evaluating JSON schemas on every request could be expensive. Mitigation: Cache compiled schemas per example.

**Expression injection**: Runtime expressions allow reading request data; need to ensure they cannot be used for unintended side effects. Mitigation: Sandbox expression evaluation to only allowed data sources.

**State isolation**: Sharing state across schemas could cause unintended interference. Mitigation: Strict namespace separation based on schema origin.

**Large request history**: Storing unlimited history could consume memory. Mitigation: Configurable limit (default 1000 entries) with FIFO eviction.

**Dependency stability**: Relying on third‑party libraries (`kin-openapi`, `chi`) introduces upgrade risk. Mitigation: Pin versions and monitor for breaking changes.

**OpenAPI extension compatibility**: The custom extensions may conflict with future OpenAPI tooling. Mitigation: Keep extensions under `x‑` prefix as per OpenAPI specification.

## Migration Plan

Not applicable – this is a new implementation with no existing users to migrate.

## Open Questions

1. Should the runtime expression engine support custom user‑defined modifiers?
 
   **Answer**: No, but it should be easy extendable with code by modularity of design 

2. How to handle nested JSON Schema references in `x-mock-match`? 
   
   **Answer**: Use the same resolver as the main OpenAPI schema.   
   
3. Should the management API be protected by a simple token or basic auth?
   
   **Answer**: No, but it can be switched off by CLI option `--no-control-api` or env OASMOCK_NO_CONTROL_API
 
4. What is the maximum size for request history? 

   **Answer**: By default - 1000 items, configurable by 

5. How to handle file‑system paths in `--from` when running inside a container?
