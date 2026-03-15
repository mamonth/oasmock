## ADDED Requirements

### Requirement: CLI command structure
The CLI SHALL provide a command-line interface with global options and subcommands as described in [cli.md](../../../../../cli.md).

#### Scenario RS.CLI.1: Invoking oasmock without arguments
- **WHEN** user runs `oasmock` without arguments
- **THEN** the mock command is executed with default options

#### Scenario RS.CLI.2: Showing version
- **WHEN** user runs `oasmock --version` or `oasmock -v`
- **THEN** the tool prints version information and exits with code 0

#### Scenario RS.CLI.3: Showing global help
- **WHEN** user runs `oasmock --help` or `oasmock -h`
- **THEN** the tool prints global help text describing available commands and options

### Requirement: Mock command
The mock command SHALL start an HTTP mock server based on OpenAPI schema(s) using extensions described in extensions.md.

#### Scenario RS.CLI.4: Starting mock server with default schema
- **WHEN** user runs `oasmock`
- **THEN** the server starts listening on port 19191 with schema from src/openapi.yaml

#### Scenario RS.CLI.5: Starting mock server with custom port
- **WHEN** user runs `oasmock --port 8080`
- **THEN** the server starts listening on port 8080

#### Scenario RS.CLI.6: Starting mock server with multiple schemas
- **WHEN** user runs `oasmock --from api/v1/openapi.yaml --prefix /v1 --from api/v2/openapi.yaml --prefix /v2`
- **THEN** the server loads both schemas and routes requests under the respective prefixes

#### Scenario RS.CLI.7: Adding request-response delay
- **WHEN** user runs `oasmock --delay 500`
- **THEN** the server introduces a 500ms delay before sending each response

#### Scenario RS.CLI.8: Enabling verbose logging
- **WHEN** user runs `oasmock --verbose`
- **THEN** the server logs detailed request/response information to stdout

#### Scenario RS.CLI.9: Disabling CORS
- **WHEN** user runs `oasmock --nocors`
- **THEN** the server does not add CORS headers to responses

#### Scenario RS.CLI.10: Showing mock command help
- **WHEN** user runs `oasmock --help` or `oasmock -h`
- **THEN** the tool prints help text for the mock command including all options

### Requirement: Environment variable overrides
The CLI SHALL support environment variables that override command-line options.

#### Scenario RS.CLI.11: Overriding port via environment
- **WHEN** OASMOCK_PORT=9090 and user runs `oasmock`
- **THEN** the server starts listening on port 9090

#### Scenario RS.CLI.12: Overriding verbose logging via environment
- **WHEN** OASMOCK_VERBOSE=true and user runs `oasmock`
- **THEN** the server enables verbose logging

#### Scenario RS.CLI.13: Overriding CORS via environment
- **WHEN** OASMOCK_NO_CORS=true and user runs `oasmock`
- **THEN** the server disables CORS headers

### Requirement: Exit codes
The CLI SHALL return appropriate exit codes as defined in [cli.md](../../../../../cli.md).

#### Scenario RS.CLI.14: Successful execution
- **WHEN** the mock server starts successfully
- **THEN** the CLI exits with code 0

#### Scenario RS.CLI.15: Invalid command-line arguments
- **WHEN** user provides invalid command-line arguments
- **THEN** the CLI exits with code 2

#### Scenario RS.CLI.16: Schema loading or validation failure
- **WHEN** the OpenAPI schema cannot be loaded or is invalid
- **THEN** the CLI exits with code 3

#### Scenario RS.CLI.17: Port already in use
- **WHEN** the requested port is already occupied
- **THEN** the CLI exits with code 4

#### Scenario RS.CLI.18: General error
- **WHEN** any other error occurs
- **THEN** the CLI exits with code 1
