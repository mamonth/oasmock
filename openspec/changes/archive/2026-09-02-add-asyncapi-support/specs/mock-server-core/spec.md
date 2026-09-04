# Mock Server Core Delta

## Purpose

Extend the core mock server so schema loading and routing accept AsyncAPI 3.x specifications (autodetected) in addition to OpenAPI, while preserving prefixing, state isolation, history, CORS, and delay behavior.

## MODIFIED Requirements

### Requirement: OpenAPI schema loading
The mock server SHALL load one or more OpenAPI 3.x or AsyncAPI 3.x schemas (auto-detected by the loader) from files specified via the `--from` CLI option, pairing each with an optional path prefix.

#### Scenario RS.MSC.1: Loading a single schema
- **WHEN** the server starts with `--from api/openapi.yaml`
- **THEN** the server parses the YAML file and validates it as OpenAPI (3.1 or 3.0)

#### Scenario RS.MSC.2: Loading multiple schemas with prefixes
- **WHEN** the server starts with `--from v1.yaml --prefix /v1 --from v2.yaml --prefix /v2`
- **THEN** the server loads both schemas and routes requests under the respective path prefixes

#### Scenario RS.MSC.3: Schema validation failure
- **WHEN** the specified file is not a valid OpenAPI or AsyncAPI 3.x schema
- **THEN** the server fails to start and exits with code 3

#### Scenario RS.MSC.50: Loading an AsyncAPI schema alongside OpenAPI
- **WHEN** the server starts with `--from openapi.yaml --from asyncapi.yaml`
- **THEN** the server auto-detects and loads both specifications through their respective loaders

#### Scenario RS.MSC.51: AsyncAPI channels honor schema prefix
- **WHEN** an AsyncAPI schema is loaded with a prefix
- **THEN** its channel addresses are served under that prefix

### Requirement: Request routing
The mock server SHALL route incoming traffic to the matching OpenAPI path or AsyncAPI channel based on method and path pattern for HTTP, and channel address for ws, so that only defined operations are served.

#### Scenario RS.MSC.7: No matching operation
- **WHEN** a request arrives at a path/method not defined in any loaded schema
- **THEN** the server responds with HTTP 404

#### Scenario RS.MSC.52: Routing an AsyncAPI HTTP channel
- **WHEN** an HTTP request arrives matching an AsyncAPI channel with an `http` binding
- **THEN** the server selects the corresponding operation for processing

#### Scenario RS.MSC.53: Routing an AsyncAPI ws channel
- **WHEN** a ws client connects to a channel address defined in an AsyncAPI schema
- **THEN** the server selects the corresponding receive/send operation for processing