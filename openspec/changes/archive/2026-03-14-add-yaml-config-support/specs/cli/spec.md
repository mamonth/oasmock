## ADDED Requirements

### Requirement: Configuration file support
The CLI SHALL read configuration from a `.oasmock.yaml` file in the current working directory (or user home directory). The configuration file SHALL use simplified schema configuration keys: `schema` (single string) for one schema, `schemas` (list) for multiple schemas. Each element in `schemas` SHALL be either a string (schema path) or an object with `src` and optional `prefix`. Other options SHALL use kebab‑case keys matching CLI flag names (`port`, `delay`, `verbose`, `nocors`, `history‑size`, `no‑control‑api`).

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

#### Scenario RS.CLI.24: Single schema configuration
- **WHEN** a `.oasmock.yaml` file contains:
  ```yaml
  schema: ../some/path/openapi.yaml
  port: 8080
  ```
- **THEN** the CLI loads the single schema from the specified path, as if `--from ../some/path/openapi.yaml` were given on the command line

#### Scenario RS.CLI.25: Multiple schemas configuration
- **WHEN** a `.oasmock.yaml` file contains:
  ```yaml
  schemas:
    - src: ../some/path/openapi.yaml
      prefix: /url/prefix
    - ../path/unprefixed.openapi.yaml
  ```
- **THEN** the CLI loads both schemas, the first with prefix `/url/prefix` and the second without prefix, as if `--from ../some/path/openapi.yaml --prefix /url/prefix --from ../path/unprefixed.openapi.yaml` were given on the command line

#### Scenario RS.CLI.26: Invalid schema configuration (both schema and schemas)
- **WHEN** a `.oasmock.yaml` file contains both `schema` and `schemas` keys
- **THEN** the CLI reports an error and exits with code 2 (invalid command-line arguments)

#### Scenario RS.CLI.27: Invalid schemas list element
- **WHEN** a `.oasmock.yaml` file contains a `schemas` list with an element that is neither a string nor an object with `src`
- **THEN** the CLI reports an error and exits with code 2 (invalid command-line arguments)

#### Scenario RS.CLI.28: CLI flag overrides YAML schema configuration
- **WHEN** a `.oasmock.yaml` file contains `schema: path/to/schema.yaml` and the user runs `oasmock --from other.yaml`
- **THEN** the CLI loads `other.yaml` (ignoring the YAML schema configuration)

#### Scenario RS.CLI.29: CLI flag overrides YAML schemas configuration
- **WHEN** a `.oasmock.yaml` file contains `schemas:` list with multiple schemas and the user runs `oasmock --from single.yaml`
- **THEN** the CLI loads `single.yaml` (ignoring the YAML schemas configuration)

## MODIFIED Requirements

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