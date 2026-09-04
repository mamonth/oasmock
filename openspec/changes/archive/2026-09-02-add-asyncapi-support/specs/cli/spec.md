# CLI Delta

## Purpose

The `--from`/`schemas` configuration and schema-failure handling now apply to AsyncAPI 3.x specifications as well as OpenAPI, with no new flags required.

## MODIFIED Requirements

### Requirement: Mock command
The mock command SHALL start a mock server based on OpenAPI and/or AsyncAPI schema(s), using the extensions described in extensions.md.

#### Scenario RS.CLI.4: Starting mock server with default schema
- **WHEN** user runs `oasmock`
- **THEN** the server starts listening on port 19191 with schema from src/openapi.yaml

#### Scenario RS.CLI.6: Starting mock server with multiple schemas
- **WHEN** user runs `oasmock --from api/v1/openapi.yaml --prefix /v1 --from api/v2/openapi.yaml --prefix /v2`
- **THEN** the server loads both schemas and routes requests under the respective prefixes

#### Scenario RS.CLI.30: Starting mock server with an AsyncAPI spec
- **WHEN** user runs `oasmock --from api/asyncapi.yaml`
- **THEN** the server auto-detects the AsyncAPI 3.x spec and serves its channels

#### Scenario RS.CLI.31: Mixing OpenAPI and AsyncAPI via config file
- **WHEN** a `.oasmock.yaml` file contains:
  ```yaml
  schemas:
    - src: openapi.yaml
    - src: asyncapi.yaml
  ```
- **THEN** the CLI loads both, auto-detecting each specification type

### Requirement: Exit codes
The CLI SHALL return appropriate exit codes as defined in cli.md, extending the schema-failure code to AsyncAPI specifications.

#### Scenario RS.CLI.14: Successful execution
- **WHEN** the mock server starts successfully
- **THEN** the CLI exits with code 0

#### Scenario RS.CLI.16: Schema loading or validation failure
- **WHEN** an OpenAPI or AsyncAPI schema cannot be loaded or is invalid
- **THEN** the CLI exits with code 3

#### Scenario RS.CLI.17: Port already in use
- **WHEN** the requested port is already occupied
- **THEN** the CLI exits with code 4