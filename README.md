# OASMock – OpenAPI Mock Server

![CI](https://github.com/mamonth/oasmock/actions/workflows/go.yml/badge.svg)
![code coverage](https://img.shields.io/badge/coverage-70%25-yellow)
![spec coverage](https://img.shields.io/badge/spec_coverage-100%25-brightgreen)
![Go](https://img.shields.io/badge/go-1.23+-00ADD8)

A Go‑based mock server that leverages OpenAPI 3.0 schemas enhanced with custom extensions for conditional examples, state management, and runtime expressions.

## Features

- Loads one or more OpenAPI 3.0 YAML/JSON files (with optional path prefixes)
- Supports custom extensions (`x‑mock‑params‑match`, `x‑mock‑skip`, `x‑mock‑once`, `x‑mock‑set‑state`, `x‑mock‑headers`)
- Runtime expressions (`{$request.path.id}`, `{$state.counter}`, `{$env.VAR}`) with modifiers (`default`, `getByPath`, `toJWT`)
- In‑memory state per namespace (get/set/increment/delete)
- Request history ring buffer with filtering via management API
- Dynamic example injection at runtime via HTTP API
- Configurable request delay, CORS, verbose logging
- Single binary, zero dependencies

## Installation

### From source

```bash
git clone https://github.com/mamonth/oasmock
cd oasmock
go install ./cmd/oasmock
```

### Download binary

Pre‑built binaries for Linux, macOS and Windows are available on the [Releases](https://github.com/mamonth/oasmock/releases) page.

## Quick Start

1. Create an OpenAPI schema (`api.yaml`) with at least one endpoint:

```yaml
openapi: 3.0.3
info:
  title: Sample API
  version: 1.0.0
paths:
  /hello:
    get:
      responses:
        200:
          description: OK
          content:
            application/json:
              examples:
                default:
                  value:
                    message: Hello, world!
```

2. Start the mock server:

```bash
oasmock --from api.yaml --port 8080 --verbose
```

3. Send a request:

```bash
curl http://localhost:8080/hello
# {"message":"Hello, world!"}
```

## OpenAPI Extensions

OASMock adds several custom extensions to OpenAPI example objects. Full documentation is available in [extensions.md](./extensions.md).

### `x‑mock‑params‑match`

Selects the example when the request matches the given conditions.

```yaml
examples:
  admin:
    x‑mock‑params‑match:
      '{$request.header.role}': admin
    value:
      message: Welcome, admin!
```

### `x‑mock‑skip`

Skips the example (useful for temporarily disabling an example).

### `x‑mock‑once`

Makes the example one‑time only (removed after first match).

### `x‑mock‑set‑state`

Updates server‑side state that can be referenced later via `{$state.*}`.

```yaml
x‑mock‑set‑state:
  counter:
    increment: 1
  'user-{$request.path.id}':
    value: '{$request.body.name}'
```

### `x‑mock‑headers`

Sets response headers (supports runtime expressions in values).

## Runtime Expressions

Runtime expressions are enclosed in `{$...}` and can appear in extension keys, values, and response bodies.

### Data Sources

- `{$request.path.param}`
- `{$request.query.param}`
- `{$request.header.name}`
- `{$request.cookie.name}`
- `{$request.body.field}`
- `{$state.key}`
- `{$env.VARIABLE}`

### Modifiers

- `{$request.query.id|default:unknown}` – provides a default value if the expression cannot be resolved
- `{$state.object|getByPath:deep.nested.value}` – traverses an object
- `{$state.payload|toJWT}` – (stub) encodes the value as a JWT

Embedded expressions are supported:

```yaml
value:
  url: "/api/users/{$request.path.id}/profile"
```

## Management API

The server exposes a control HTTP API under the `/_mock` prefix.

### `GET /_mock/requests`

Retrieves the request history (optionally filtered by path, method, time range, etc.).

### `POST /_mock/examples`

Adds a dynamic example to an existing route. The request body follows the schema defined in [openapi.yaml](./api/openapi.yaml).

## Command‑Line Interface

See [cli.md](./cli.md) for the complete CLI specification.

### Examples

```bash
# Multiple schemas with prefixes
oasmock \
  --from api/v1/openapi.yaml --prefix /v1 \
  --from api/v2/openapi.yaml --prefix /v2 \
  --port 19191 --delay 500 --verbose

# Disable CORS and management API
oasmock --from api.yaml --nocors --no-control-api

# Environment variable overrides
export OASMOCK_PORT=9999
export OASMOCK_VERBOSE=true
oasmock --from api.yaml
```

## Development

### Building

```bash
go build ./cmd/oasmock
```

### Testing

```bash
go test ./...
```

### Linting

```bash
golangci-lint run
```

## License

MIT