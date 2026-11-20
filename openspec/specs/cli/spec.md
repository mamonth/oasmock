## Purpose

Command-line interface for the OASMock tool. Provides a single binary (`oasmock`) that starts an HTTP mock server based on OpenAPI schema(s) with configurable options and environment variable overrides.
## Requirements
### Requirement: CLI command structure
The CLI SHALL provide a command-line interface with global options and subcommands as described in [cli.md](../../../cli.md).

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
The mock command SHALL start a mock server based on OpenAPI and/or AsyncAPI schema(s), using the extensions described in extensions.md.

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

#### Scenario RS.CLI.32: Binding an ephemeral port
- **WHEN** user runs `oasmock --port 0`
- **THEN** the server binds an OS-assigned port and logs the actual bound port under `port=`

### Requirement: Environment variable overrides
The CLI SHALL support configuration sources with the following precedence: command-line arguments > environment variables > configuration file > defaults. Environment variables SHALL override configuration file values but be overridden by command-line arguments.

#### Scenario RS.CLI.11: Overriding port via environment
- **WHEN** OASMOCK_PORT=9090 and user runs `oasmock`
- **THEN** the server starts listening on port 9090 (unless a CLI flag overrides)

#### Scenario RS.CLI.12: Overriding verbose logging via environment
- **WHEN** OASMOCK_VERBOSE=true and user runs `oasmock`
- **THEN** the server enables verbose logging (unless a CLI flag overrides)

#### Scenario RS.CLI.13: Overriding CORS via environment
- **WHEN** OASMOCK_NO_CORS=true and user runs `oasmock`
- **THEN** the server disables CORS headers (unless a CLI flag overrides)

### Requirement: Exit codes
The CLI SHALL return appropriate exit codes as defined in cli.md, extending the schema-failure code to AsyncAPI specifications.

#### Scenario RS.CLI.14: Successful execution
- **WHEN** the mock server starts successfully
- **THEN** the CLI exits with code 0

#### Scenario RS.CLI.15: Invalid command-line arguments
- **WHEN** user provides invalid command-line arguments
- **THEN** the CLI exits with code 2

#### Scenario RS.CLI.16: Schema loading or validation failure
- **WHEN** an OpenAPI or AsyncAPI schema cannot be loaded or is invalid
- **THEN** the CLI exits with code 3

#### Scenario RS.CLI.17: Port already in use
- **WHEN** the requested port is already occupied
- **THEN** the CLI exits with code 4

#### Scenario RS.CLI.18: General error
- **WHEN** any other error occurs
- **THEN** the CLI exits with code 1

### Requirement: Configuration file support
The CLI SHALL read configuration from a `.oasmock.yaml` file in the current working directory (or user home directory). The configuration file SHALL use the `schemas` list key for schema configuration. Each element in `schemas` SHALL be either a string (schema path) or an object with `src` and optional `prefix`. Other options SHALL use kebab‑case keys matching CLI flag names (`port`, `delay`, `verbose`, `nocors`, `history‑size`, `no‑control‑api`).

#### Scenario RS.CLI.19: Config file present with valid YAML
- **WHEN** a `.oasmock.yaml` file exists in the current directory with valid YAML content
- **THEN** the CLI uses the values from the file as defaults (unless overridden by environment variables or CLI flags)

#### Scenario RS.CLI.20: Config file missing
- **WHEN** no `.oasmock.yaml` file exists in the current directory or user home directory
- **THEN** the CLI proceeds without error, using environment variables and CLI flags as usual

#### Scenario RS.CLI.21: Config file malformed
- **WHEN** a `.oasmock.yaml` file exists but contains invalid YAML syntax
- **THEN** the CLI logs a warning and proceeds without configuration file values

#### Scenario RS.CLI.22: Precedence - CLI flag overrides config file
- **WHEN** a configuration value is defined both in `.oasmock.yaml` and as a CLI flag (e.g., `port: 8080` in YAML and `--port 9090` on command line)
- **THEN** the CLI uses the value from the CLI flag

#### Scenario RS.CLI.23: Precedence - environment variable overrides config file
- **WHEN** a configuration value is defined both in `.oasmock.yaml` and as an environment variable (e.g., `port: 8080` in YAML and `OASMOCK_PORT=7070`)
- **THEN** the CLI uses the value from the environment variable (unless overridden by a CLI flag)

#### Scenario RS.CLI.25: Multiple schemas configuration
- **WHEN** a `.oasmock.yaml` file contains:
  ```yaml
  schemas:
    - src: ../some/path/openapi.yaml
      prefix: /url/prefix
    - ../path/unprefixed.openapi.yaml
  ```
- **THEN** the CLI loads both schemas, the first with prefix `/url/prefix` and the second without prefix, as if `--from ../some/path/openapi.yaml --prefix /url/prefix --from ../path/unprefixed.openapi.yaml` were given on the command line

#### Scenario RS.CLI.27: Invalid schemas list element
- **WHEN** a `.oasmock.yaml` file contains a `schemas` list with an element that is neither a string nor an object with `src`
- **THEN** the CLI reports an error and exits with code 2 (invalid command-line arguments)

#### Scenario RS.CLI.29: CLI flag overrides YAML schemas configuration
- **WHEN** a `.oasmock.yaml` file contains `schemas:` list with multiple schemas and the user runs `oasmock --from single.yaml`
- **THEN** the CLI loads `single.yaml` (ignoring the YAML schemas configuration)

### Requirement: TLS command-line options
The mock command SHALL accept `--tls-cert`, `--tls-key` and `--tls-port` options, enable the additional TLS listener only when both certificate and key are provided, and default the TLS port to `8443`.

#### Scenario RS.CLI.33: Starting with TLS certificate and key
- **WHEN** user runs `oasmock --tls-cert server.crt --tls-key server.key`
- **THEN** the plain HTTP listener starts on its configured port and an additional HTTPS listener starts on port 8443

#### Scenario RS.CLI.34: Selecting a custom TLS port
- **WHEN** user runs `oasmock --tls-cert server.crt --tls-key server.key --tls-port 9443`
- **THEN** the additional HTTPS listener starts on port 9443

#### Scenario RS.CLI.35: TLS omitted
- **WHEN** user runs `oasmock` without TLS certificate and key options
- **THEN** only the plain HTTP listener starts

#### Scenario RS.CLI.36: Binding an ephemeral TLS port
- **WHEN** user runs `oasmock --tls-cert server.crt --tls-key server.key --tls-port 0`
- **THEN** the TLS listener binds an OS-assigned port and logs the actual TLS port after startup

### Requirement: TLS configuration sources
The CLI SHALL read TLS certificate, key and port from `OASMOCK_TLS_CERT`, `OASMOCK_TLS_KEY`, `OASMOCK_TLS_PORT` environment variables and from the `tls_cert`, `tls_key`, `tls_port` configuration-file keys, using the existing precedence: CLI arguments > environment variables > configuration file > defaults.

#### Scenario RS.CLI.37: TLS options from configuration file
- **WHEN** `.oasmock.yaml` contains `tls_cert`, `tls_key` and `tls_port`
- **THEN** the CLI starts the HTTPS listener with those values unless overridden

#### Scenario RS.CLI.38: TLS environment override and flag precedence
- **WHEN** TLS values are defined in the configuration file, environment, and CLI flags
- **THEN** the environment overrides the configuration file and the CLI flag overrides the environment

### Requirement: TLS configuration validation
The CLI SHALL reject incomplete or conflicting TLS configuration before starting the server, returning exit code 2 for invalid command-line arguments or configuration.

#### Scenario RS.CLI.39: Only one certificate component
- **WHEN** user provides `--tls-cert` without `--tls-key`, or `--tls-key` without `--tls-cert`
- **THEN** the CLI reports an error and exits with code 2

#### Scenario RS.CLI.40: TLS port without certificate pair
- **WHEN** user sets a TLS port through a flag, environment variable, or configuration file without providing both certificate and key
- **THEN** the CLI reports an error and exits with code 2

#### Scenario RS.CLI.41: TLS port conflicts with HTTP port
- **WHEN** the configured TLS port equals the plain HTTP port
- **THEN** the CLI reports an error and exits with code 2

### Requirement: TLS startup failures
The CLI SHALL report TLS startup failures with dedicated exit codes and SHALL log both listener ports only after both listeners bind successfully.

#### Scenario RS.CLI.42: Invalid TLS certificate or key
- **WHEN** the configured certificate or key cannot be loaded or does not match
- **THEN** the CLI reports an error and exits with code 5

#### Scenario RS.CLI.43: TLS port already in use
- **WHEN** the configured TLS port is already occupied
- **THEN** the CLI reports an error and exits with code 4

#### Scenario RS.CLI.44: Successful dual-listener startup logging
- **WHEN** both listeners bind successfully
- **THEN** the CLI logs the plain HTTP port and the actual TLS port before serving continues

