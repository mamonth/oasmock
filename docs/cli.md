# CLI Specification

## Command Overview

Starts an HTTP server that mocks endpoints defined in one or more OpenAPI files, using the extensions described in [extensions.md](./extensions.md).

### Syntax

```bash
oasmock [options]
```

### Options

| Option             | Type      | Default            | Description                                                              |
|--------------------|-----------|--------------------|--------------------------------------------------------------------------|
| `--from`           | string    | `src/openapi.yaml` | Source OpenAPI or AsyncAPI schema (autodetected by root key). Can be specified multiple times.                  |
| `--prefix`         | string    | `''`               | URI prefix for the schema. Can be specified for each `--from` parameter. |
| `--port`           | number    | `19191`            | Port to listen on. Use `0` to bind an OS-assigned (ephemeral) port; the actual bound port is logged under `port=` after startup. |
| `--delay`          | number    | `100`              | Delay between request and response in milliseconds.                      |
| `--verbose`        | boolean   | `false`            | Enable verbose logging.                                                  |
| `--nocors`         | boolean   | `false`            | Disable automatic CORS compliance.                                       |
| `--no-control-api` | boolean   | `false`            | Disable management HTTP API served at _mock/                             |
| `--history-size`   | number    | `1000`             | Maximum number of requests to keep in history.                           |
| `--version`, `-v`  | boolean   | `false`            | Show version information and exit.                                       |
| `--help`, `-h`     | boolean   | `false`            | Show global help and exit.                                               |

### Environment Variables

All values are overridable by their CLI option counterparts.

| Variable                 | Description                         |
|--------------------------|-------------------------------------|
| `OASMOCK_PORT`           | Port to listen on.                  |
| `OASMOCK_VERBOSE`        | If `true`, enables verbose logging. |
| `OASMOCK_NO_CORS`        | If `true`, disables CORS.           |
| `OASMOCK_NO_CONTROL_API` | Disable management HTTP API.        |
| `OASMOCK_HISTORY_SIZE`   | Maximum request history size.       |

### Configuration File

The CLI can read configuration from a `.oasmock.yaml` file in the current working directory (or user's home directory as a fallback). Configuration file values are overridden by environment variables, which are overridden by command‑line arguments.

**Precedence:** CLI arguments > Environment variables > Configuration file > Defaults

**Location:**
- Current working directory (`.oasmock.yaml`)
- User's home directory (`~/.oasmock.yaml`)

**Format:** YAML with the following keys:

| Key               | Type                | Description                                                              |
|-------------------|---------------------|--------------------------------------------------------------------------|
| `schemas`         | list                | Multiple schemas (OpenAPI or AsyncAPI — autodetected), each either a string (path) or object with `src` and optional `prefix`. |
| `port`            | number              | Port to listen on.                                                       |
| `delay`           | number              | Delay between request and response in milliseconds.                      |
| `verbose`         | boolean             | Enable verbose logging.                                                  |
| `nocors`          | boolean             | Disable automatic CORS compliance.                                       |
| `history_size`    | number              | Maximum number of requests to keep in history.                           |
| `no_control_api`  | boolean             | Disable the management control API.                                      |

**Schema types:** each `--from`/`schemas` entry may reference an OpenAPI (`openapi:` root key) or AsyncAPI (`asyncapi:` root key, 3.0.0/3.1.0) file. Spec type is detected automatically from the root version key — no flags change. Mixed OpenAPI + AsyncAPI sources can be served together.

**Examples:**

**Multiple schemas with prefixes:**
```yaml
schemas:
  - src: api/v1/openapi.yaml
    prefix: /v1
  - api/v2/openapi.yaml
port: 8080
delay: 500
nocors: true
```

### Examples

**Start mock server with default params**:
```bash
oasmock
```

**Start on a custom port with two schemas**:
```bash
oasmock --port 8080 \
  --from api/v1/openapi.yaml \
  --prefix /v1 \
  --from api/v2/openapi.yaml \
  --prefix /v2
```

**Add delay and verbose output**:
```bash
oasmock --delay 500 --verbose
```

**Disable CORS**:
```bash
oasmock --nocors
```

## Exit Codes

| Code | Meaning                               |
|------|---------------------------------------|
| 0    | Success                               |
| 1    | General error                         |
| 2    | Invalid command‑line arguments        |
| 3    | Schema loading or validation failed   |

