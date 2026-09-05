# OASMock – OpenAPI Mock Server

![CI](https://github.com/mamonth/oasmock/actions/workflows/go.yml/badge.svg)
![code coverage](https://img.shields.io/badge/coverage-70%25-yellow)
![spec coverage](https://img.shields.io/badge/spec_coverage-100%25-brightgreen)
![Go](https://img.shields.io/badge/go-1.23+-00ADD8)

A Go‑based mock server that leverages OpenAPI 3.0 schemas enhanced with custom extensions for conditional examples, state management, runtime expressions, and JSON-RPC 2.0 support.

## Features

- Loads one or more OpenAPI 3.0 YAML/JSON files (with optional path prefixes)
- Supports custom extensions (`x‑mock‑match`, `x‑mock‑skip`, `x‑mock‑once`, `x‑mock‑set‑state`, `x‑mock‑headers`; legacy `x‑mock‑params‑match` alias still supported)
- Runtime expressions (`{$request.path.id}`, `{$state.counter}`, `{$env.VAR}`) with modifiers (`default`, `getByPath`, `toJWT`)
- In‑memory state per namespace (get/set/increment/delete)
- Request history ring buffer with filtering via management API
- Dynamic example injection at runtime via HTTP API
- Configurable request delay, CORS, verbose logging
- Single static binary, no runtime dependencies
- JSON‑RPC 2.0 gateway via `x‑rpc` extension (batch requests, notifications)

## Installation

### From source

```bash
git clone https://github.com/mamonth/oasmock
cd oasmock
go install ./cmd/oasmock
```

### Download binary

Pre‑built binaries for Linux, macOS and Windows are available on the [Releases](https://github.com/mamonth/oasmock/releases) page.

### Docker

```bash
docker pull itmamonth/oasmock:latest
```

Run with a mounted `.oasmock.yaml` config and your OpenAPI schemas:

```bash
docker run -v $(pwd)/.oasmock.yaml:/app/.oasmock.yaml \
           -v $(pwd)/schemas:/schemas:ro \
           -p 8080:8080 \
           itmamonth/oasmock:latest
```

See [docs/docker.md](./docs/docker.md) for configuration, Docker Compose, image tags, and multi‑platform usage.

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

OASMock adds several custom extensions to OpenAPI example objects. Full reference: [extensions.md](./docs/extensions.md).

### Match Conditions (`x‑mock‑match`)

Selects the example when the request matches the given conditions (deprecated alias: `x‑mock‑params‑match`).

```yaml
examples:
  admin:
    x‑mock‑match:
      '{$request.header.role}': admin
    value:
      message: Welcome, admin!
```

### Other Extensions

| Extension | Purpose |
|---|---|
| `x‑mock‑skip` | Temporarily exclude an example |
| `x‑mock‑once` | One‑time example (removed after first match) |
| `x‑mock‑set‑state` | Update server‑side state (supports `increment`, `value`, `null` for delete) |
| `x‑mock‑headers` | Set response headers (runtime expressions in values) |

### JSON‑RPC Gateway (`x‑rpc`)

Route calls by body field instead of URL path. See [json-rpc.md](./docs/json-rpc.md).

## Runtime Expressions

Runtime expressions are enclosed in `{$...}` and resolved at request time. Data sources: `{$request.path.param}`, `{$request.query.param}`, `{$request.header.name}`, `{$request.body.field}`, `{$request.cookie.name}`, `{$state.key}`, `{$env.VARIABLE}`, and for async-driven examples `{$event.name}`/`{$event.data}`/`{$event.<field>}` plus per-connection `{$connection.id}`/`{$connection.channel}`/`{$connection.query.<key>}`/`{$connection.header.<key>}`.

Modifiers: `\|default:value` (fallback), `\|getByPath:path` (traverse nested objects), `\|toJWT` (stub).

Expressions can appear in extension keys, values, and response bodies. Full reference: [extensions.md](./docs/extensions.md#runtime-expressions).

## Management API

The server exposes a control HTTP API under the `/_mock` prefix. Full schema: [api/openapi.yaml](./api/openapi.yaml). The asynchronous control surface (the management WebSocket stream `/_mock/stream` and its envelopes) is described in [api/asyncapi.yaml](./api/asyncapi.yaml). Both specs are kept in sync with the implementation by contract tests in `internal/server/control_api_spec_sync_test.go`.

- `GET /_mock/requests` — request history (filterable by path, method, time range, pagination)
- `POST /_mock/examples` — add a dynamic example to an existing route
  - sync (OpenAPI) targets use `path`; AsyncAPI targets use `channel` with optional `match`/`interval`/`delay` mirroring `x-mock-match`/`x-mock-interval`/`x-mock-delay` for live event-driven or recurring delivery
- `DELETE /_mock/examples/{exampleId}` — remove a dynamic example and cancel any recurring interval delivery
- `POST /_mock/events` — fire a named event ad-hoc with a `type` discriminator (`fire` for V1)
- `POST /_mock/async/push` — push a message to channel consumers (immediate/delayed, targeted/broadcast)
- `GET /_mock/async/consumers` — list connected consumers (`channel` optional, all channels when omitted)
- `POST /_mock/async/disconnect` — force-disconnect a consumer
- `GET /_mock/stream` — management WebSocket stream of runtime notifications (event/push/consumer/schedule envelopes, filtered at connect time)

The legacy `/_mock/ws/*` aliases and `/_mock/events/fire` are deprecated but still work; the removed `/_mock/ws/schedule*` answers `410 Gone` pointing at the examples endpoint.

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

## Further Reading

- [CLI reference](./docs/cli.md) — all flags, env vars, config file (`.oasmock.yaml`)
- [Extensions & runtime expressions](./docs/extensions.md) — full `x‑mock‑*` / `x‑rpc` reference
- [JSON‑RPC 2.0](./docs/json-rpc.md) — protocol details, batch support, error codes
- [Architecture](./docs/architecture.md) — component diagrams, interfaces, data flows
- [CI/CD](./docs/ci-cd.md) — pipeline, quality gates, release process
- [Project standards](./docs/project.md) — tech stack, conventions, testing, coverage policy
- [Specifications (BDD)](./openspec/specs/) — requirement scenarios

## License

MIT
