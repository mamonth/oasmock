# Spec Delta

## ADDED Requirements

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
