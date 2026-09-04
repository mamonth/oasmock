## Why

OASMock currently only understands OpenAPI patterns, limiting its usefulness for teams building event-driven and real-time APIs. AsyncAPI is the de-facto standard for describing event-driven systems (WebSockets, message brokers). Supporting AsyncAPI 3.0.0/3.1.0 lets users mock event-driven and real-time backends with the same workflow and templating power they already get for OpenAPI — including SignalR hubs and server-initiated pushes that real-time consumers rely on.

## What Changes

- **Autodetect spec factory**: The loader detects whether a given file is an OpenAPI or AsyncAPI spec (by `openapi` vs `asyncapi` root key and version) and dispatches to the correct loader automatically — no new CLI flags required.
- **AsyncAPI loading & validation**: Load and validate AsyncAPI 3.0.0 and 3.1.0 specs from files and inline references, with the same failure semantics as OpenAPI (exit code 3).
- **MVP protocol support**: Mock channels/operations for `ws` and `http` protocol bindings. HTTP channels reuse the existing HTTP mock pipeline; ws channels get minimal MVP serving (connect, echo, receive-operation emission). Unsupported protocols (including `amqp` and `kafka`) fail startup with a clear error.
- **SignalR hub runtime**: An AsyncAPI document whose root declares `x-signalr` is served as a single ASP.NET Core SignalR hub — `negotiate`, handshake, `\x1e` framing, `StreamInvocation` → held-open `StreamItem` streams, cancel/completion, and server→client pushes. Hub streams map to channels and one-shot invocations map to operations, so all content stays native AsyncAPI.
- **Event-driven push bus**: A server-side event broker decouples producers from consumers. OpenAPI response examples fire named events via `x-event-trigger` (list form, optional payload/delay/global); AsyncAPI message examples subscribe via `x-send-events` (`on: <event>` or built-ins `receive`/`connect`/`cron`) and emit to channel consumers, templated with `{$event.*}`. REST and AsyncAPI models never reference one another; delivery is broadcast with client-side filtering.
- **Full templating parity**: AsyncAPI message examples support the complete OpenAPI templating realization — runtime expressions (`{$request.*}`, `{$state.*}`, `{$env.*}`, `{$message.*}`, `{$channel.*}`, `{$event.*}`), example selection with `x-mock-*` extensions, and dynamic example handling.
- **Management API for AsyncAPI**: `/_mock/examples` also accepts AsyncAPI channel routes (protocol + address) for dynamic example injection. A new async-mocking surface drives consumers at runtime: delayed push, targeted/broadcast push, connected-consumer discovery (connection IDs and open streams), templated push payloads, recurring scheduled push, a fire-event endpoint, and connection lifecycle control (force disconnect / simulate drop).
- **Isolated namespaces**: Each AsyncAPI spec keeps its own state namespace and prefix behavior, consistent with multi-schema OpenAPI support; events are schema-local unless declared `global: true`.
- **Non-goal for this change**: AMQP serving; Binance diff-depth book (U/u continuity, zero-qty deletes, snapshot re-bootstrap) — streaming sequence/pacing is provided via state + `cron` send-events; SignalR half-transports (SSE/LongPolling), MessagePack, and Ack/Sequence; per-connection session/account routing.

## Capabilities

### New Capabilities
- `asyncapi-loader`: Auto-detect AsyncAPI vs OpenAPI specs and load/validate AsyncAPI 3.0.0 & 3.1.0.
- `asyncapi-protocols`: Map AsyncAPI channels/operations/messages to runnable mocks for `ws` and `http` protocol bindings (MVP); unsupported protocols fail startup.
- `signalr-hub-runtime`: Serve documents with root `x-signalr` as ASP.NET Core SignalR hubs — negotiate, handshake, `\x1e` framing, held-open streams (channels) and one-shot invocations (operations), server pushes.
- `event-driver`: Event bus — `x-event-trigger` on OpenAPI examples and `x-send-events` on AsyncAPI message examples, with `{$event.*}` templating, schema-local/global scoping, and a management fire-event endpoint.
- `asyncapi-templating`: Reuse the full runtime-expression + `x-mock-*` extension + state/history pipeline for AsyncAPI message examples.
- `asyncapi-management`: Management API for driving async mocking — delayed push delivery, targeted/broadcast push, consumer discovery, templated push payloads, recurring scheduled push, fire-event, and connection lifecycle control (force disconnect / simulate drop).

### Modified Capabilities
- `mock-server-core`: Schema loading and request routing requirements are extended to accept AsyncAPI specs (autodetected) in addition to OpenAPI, retaining prefixing, state isolation, history, CORS, and delay behavior.
- `cli`: `--from`/config file `schemas` entries and schema-failure exit-code behavior now apply to AsyncAPI specs too.
- `management-api`: Dynamic example injection (`/_mock/examples`) accepts AsyncAPI channel routes in addition to OpenAPI path/method routes.

## Impact

- `internal/loader/schema.go` — introduce autodetect factory; add AsyncAPI load path + validation.
- `internal/loader/router.go` — build channel/operation mappings for AsyncAPI alongside OpenAPI path mappings.
- `internal/asyncapi/` — vendored-parser-backed neutral AsyncAPI document view, structural validation, and `x-*` extension capture (message examples + root `x-signalr`/`x-send-events`).
- `internal/server/` — new protocol adapters (ws, http), the SignalR overlay (negotiate/handshake/framing/stream registry), and an event broker (`x-event-trigger`/`x-send-events` emission); reuse of the example-selection, state, history, and management pipelines.
- `internal/server/server_management.go` — AsyncAPI route resolution for `/_mock/examples`, the async-mocking endpoints (push, schedule, consumers/streams, lifecycle), and the fire-event endpoint.
- `internal/runtime/` — message- and event-oriented data sources for expressions (`MessageSource`, `EventSource`; `{$message.*}`, `{$channel.*}`, `{$event.*}`).
- `internal/extensions/` — reuse `x-mock-*` extraction on AsyncAPI message examples (`ExampleValue` wrapper), plus `x-event-trigger` handling on OpenAPI examples.
- `cmd/oasmock/mock.go` — wire AsyncAPI specs through config without breaking flag semantics.
- `api/openapi.yaml` — add async-mocking endpoints (including fire-event) and AsyncAPI route targeting to the management API contract.
- `go.mod` — new dependency for AsyncAPI parsing (`github.com/benelser/go-asyncapi`, vendored under `third_party/` via `replace`) and the WebSocket library (`github.com/gorilla/websocket`).
- Docs: `docs/architecture.md`, `docs/project.md` (structure), and CLI docs updated.