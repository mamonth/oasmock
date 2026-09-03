# Design: AsyncAPI 3.x Support (MVP)

## Context

OASMock loads OpenAPI specs via `internal/loader` (`LoadSchemas` → `loadSingleSchema`), builds `RouteMapping`s in `internal/loader/router.go`, and serves them through the HTTP server in `internal/server`. Templating (runtime expressions `{$...}`, `x-mock-*` extensions, state, history) is built around `*openapi3.Operation`/`*openapi3.Example` types and the `runtime.Evaluator`.

The AsyncAPI ecosystem has no single canonical Go parser like `kin-openapi`. The `parser-go` official parser is archived and validates only 1.x–2.6.0 (it rejects `asyncapi: 3.x`). The viable Go option for 3.x is `github.com/benelser/go-asyncapi` (AsyncAPI 3.0.0 parser/validator with `$ref` resolution), which is young but loads/resolves both 3.0.0 and 3.1.0 documents and yields typed domain objects (channels, operations, messages, bindings, components). Because it is young it is adopted **behind an internal abstraction** so it stays cheap to swap.

This is an MVP: `ws` and `http` protocol bindings get real but minimal serving, and the `ws` surface additionally hosts a **SignalR hub overlay** (negotiate, handshake, `\x1e` framing, held-open streams) plus an **event-driven push bus** (`x-event-trigger` / `x-send-events`) that connects REST producers to ws consumers without model-to-model references. `amqp` is **not** served in this change (it is treated as an unsupported protocol). Deeper protocol fidelity (AMQP broker, SSE/LongPolling, MessagePack, Ack/Sequence) is explicitly deferred.

## Goals / Non-Goals

**Goals:**
- Auto-detect OpenAPI vs AsyncAPI files by root version key, with no new CLI flags.
- Load/validate AsyncAPI 3.0.0 and 3.1.0 via the vendored parser (behind `internal/asyncapi`), exposing channels/operations/messages/bindings.
- Map channels to runnable mock surfaces for `http` and `ws` (MVP).
- **Event-driven push** (`event-driver`): OpenAPI examples fire named events (`x-event-trigger`); AsyncAPI message examples subscribe (`x-send-events` with `on: <event>` or built-ins `receive`/`connect`/`cron`) and emit to channel consumers; event payloads templated via `{$event.*}`. REST and AsyncAPI models never reference one another.
- Serve a **SignalR hub** (official-client compatible) declared at the document root via `x-signalr`: `negotiate`, handshake, `\x1e` framing, streams map to channels, one-shot invocations map to operations, server → client pushes.
- Reuse the existing templating pipeline unchanged for AsyncAPI message examples: expressions, `x-mock-*` extensions, state, history, dynamic examples.
- Keep prefixes, state-namespace isolation, CORS, delay, verbose logging, and management API working for AsyncAPI traffic. Management additionally fires named events ad-hoc.

**Non-Goals:**
- AMQP 0-9-1 serving (`amqp` bindings fail startup like `kafka`).
- Binance-specific diff-depth book (U/u continuity, zero-qty deletes, snapshot re-bootstrap). Streaming clients needing sequence numbers/pacing use existing **state + inline templating + `cron` send-events**; a dedicated book engine is deferred.
- SSE / LongPolling (SignalR half-transports), MessagePack, `Ack`/`Sequence` (negotiate `useAck`): listed-and-declined or 400 on attempt.
- Per-connection session/account routing: event-driven delivery is **broadcast with client-side filtering**; session identity is not modeled.
- Additional AsyncAPI protocols (`kafka`, `mqtt`, `nats`, ...) — unsupported protocols produce a startup error.
- AsyncAPI 2.x or OpenAPI 2.0 (swagger) support.
- Re-implementing the runtime expression engine — it is reused as-is.

## Decisions

### D1: Use `github.com/benelser/go-asyncapi` behind an `internal/asyncapi` abstraction
Adopt `github.com/benelser/go-asyncapi` (vendored: it declares a wrong module path and requires Go 1.25) as the AsyncAPI 3.x parser, but expose it only through a thin internal abstraction (`internal/asyncapi` package):
- `internal/asyncapi` defines a **neutral `Document` view** (channels, operations, messages, bindings, examples with `x-mock-*` extensions) and a `Parse(data)` entry point — no third-party types leak past this package.
- A vendor copy (`third_party/go-asyncapi`) is wired via a `replace` directive; the vendored go.mod is corrected (module path `github.com/benelser/go-asyncapi`, `go 1.23` to match project/CI) and test files are pruned. The embedded `asyncapi-3.0.0.json` schema is kept only for ±structural checks.
- The abstraction performs its own **structural validation** for both 3.0.0 and 3.1.0 (mandatory top-level fields, channels/operations presence, unknown-protocol detection, version-major == 3). The library's bundled schema is NOT used as the source of truth because it is 3.0.0-only and rejects `x-mock-*` extensions (`additionalProperties: false`).
- `x-mock-*` extensions on message examples are captured by `internal/asyncapi` from the raw document (the vendored `MessageExample` is lightly patched to retain `x-*` keys). **Root/document-level `x-*` extensions** (`x-signalr`, and `x-send-events` on message examples) and **OpenAPI example `x-event-trigger`** are captured the same way on the neutral views.
- **Rationale**: isolates the unproven dependency behind two seams (one package imports it; one `replace` pins it), so swapping to a stricter 3.1.0 parser later only changes `internal/asyncapi` and the `replace`.
- **Alternative considered**: hand-rolled `map[string]any` model — rejected as it would duplicate ref-resolution/validation logic and drift from spec; `parser-go` — rejected (archived, no 3.x).

### D2: Spec-type autodetect factory in `internal/loader`
Replace the direct `openapi3.NewLoader()` call with a factory that reads raw bytes and dispatches on the root key:
- `openapi:` → `loadOpenAPI(data)` (existing `kin-openapi` path)
- `asyncapi:` → `loadAsyncAPI(data)` (`internal/asyncapi` path, checks major version == 3)
- neither → schema error (preserves exit code 3 and `RS.MSC.3`).

`SchemaInfo` gains a `Kind` field (`OpenAPI` | `AsyncAPI`) plus an AsyncAPI view (`*asyncapi.Document` from `internal/asyncapi`), keeping `Prefix` semantics unchanged.
- **Rationale**: centralized detection keeps `LoadSchemas` signature stable and mixes cleanly with multi-schema/prefix config.
- **Alternative considered**: file-extension sniffing — rejected (JSON/YAML has no reliable extension signal).

### D3: Unify routing behind a `SpecRoute` consumer model
Introduce a protocol-neutral route representation the server can consume regardless of source:
```go
type SpecRoute struct {
    Protocol string            // "http" | "ws"
    Address  string            // OpenAPI path pattern OR AsyncAPI channel address (prefixed)
    Method   string            // for http
    Action   string            // "send" | "receive" | "" (OpenAPI default)
    Messages []MessageSpec     // OpenAPI: examples; AsyncAPI: operation messages w/ examples
}
```
`BuildRouteMappings` keeps its signature (`[]RouteMapping`) for OpenAPI compatibility, while AsyncAPI channels produce `RouteMapping`s carrying an AsyncAPI-backed `MessageSpec` list. The server switches example resolution from `*openapi3.Example` to a small internal `ExampleValue` that wraps either source.
- **Rationale**: avoids a parallel server for AsyncAPI; one selection/state/history pipeline for both spec kinds.
- **Alternative considered**: separate AsyncAPI server component — rejected (duplicates history/state/management logic; breaks cohesion goals).

### D4: Protocol adapters as strategies (ws/http + SignalR overlay)
A small `ProtocolAdapter` interface drives per-protocol serving:
```go
type ProtocolAdapter interface {
    Serve(ctx context.Context, route SpecRoute, handler MessageHandler) error
}
```
- `httpAdapter`: plain HTTP routes (nearly free reuse of existing pipeline).
- `wsAdapter`: WebSocket upgrade endpoint; MVP = accept connection, echo/`x-mock-*`-shaped responses, broadcast receive-operation examples. When the document declares root `x-signalr`, the ws session is handed to the **SignalR overlay** (D7) instead of raw framing.
- No `amqpAdapter` in this change — `amqp` bindings are rejected as unsupported at load (startup error, exit code 3).
- **Rationale**: strategy isolates protocol differences; MVP keeping adapters small preserves low complexity.
- **Alternative considered**: full-featured brokers / separate SignalR server library — rejected as heavy for MVP; the SignalR wire handling is small protocol code plus a spec-driven content layer.

### D5: AsyncAPI templating via reused pipeline
Templating is source-agnostic: the runtime `Evaluator` already evaluates `{$...}` expressions from named `DataSource`s. For AsyncAPI we register a `MessageSource` (payload/headers/channel params) under the existing source names, so `RS.ATM.*` scenarios pass without engine changes. Event payloads reuse the same evaluator via an event `DataSource` (`{$event.*}`) for event-driven emission (D9).
`x-mock-*` extraction (extensions/match.go, extract.go) is refactored to operate on a thin `ExampleValue` wrapper (`Payload map[string]any` + `Headers`), implemented for both OpenAPI examples and AsyncAPI message examples.
- **Rationale**: maximal reuse, minimal new logic; satisfies the "all templating support from openapi" requirement.
- **Note**: the extension matching consumes `openapi3.Example.ExtensionProps` today — the wrapper keeps that internal.

### D6: Channel parameters → expression data
AsyncAPI channel parameters (e.g. `user/{userId}`) are captured per connection/request and exposed through `{$channel.<name>}` via `MessageSource`. HTTP channel params map onto existing path-param handling.

### D7: Root-level SignalR hub runtime as a ws overlay
An AsyncAPI document with a root-level `x-signalr` extension is served as a single SignalR hub (one hub per document, mirroring the single-gateway `x-rpc` precedent):
- **Negotiate** — `POST {hubPath}/negotiate` (also `?negotiateVersion=0|1`) returns `connectionToken`, `connectionId`, `negotiateVersion`, `availableTransports` (WebSockets Text/Binary only). Unsupported transport→HTTP 400; the token is server-generated and correlated with the subsequent upgrade.
- **Handshake** — the first ws frame must be `{"protocol":"json","version":1}`; the server replies `{}\x1e`; any other content closes the connection.
- **Framing** — ws text frames are split on byte `0x1E` (record separator); each chunk is one SignalR message `{type,…}`. Types: 1 Invocation, 2 StreamItem, 3 Completion, 4 StreamInvocation, 5 CancelInvocation, 6 Ping.
- **Streams = channels**: a `StreamInvocation` (type 4) with `target` equal to a channel ID emits the channel's snapshot example as `StreamItem(s)` on the client's `invocationId` and holds the stream open. The open-stream registry tracks `(connection, invocationId, channel ID)` so event-driven pushes (D9) can append items.
- **One-shot invocations = operations**: an `Invocation` (type 1) with `target` equal to an operation ID is answered with a `Completion` carrying the operation's message example.
- **CancelInvocation / completion** → `Completion` (type 3); **server→client one-shot push** uses `Invocation` (type 1) with a server-assigned id.
- **Rationale**: official SignalR clients enforce the wire protocol (negotiate token, handshake, `\x1e` framing, envelope types, invocationId correlation) — a raw example emitter cannot satisfy them. Mapping streams→channels and invocations→operations keeps all content native AsyncAPI (no parallel hub config vocabulary), exactly as `x-rpc` maps procedures→operations.
- **Alternative considered**: per-channel `x-signalr` config — rejected as over-expanded (hub/targets/push/invocations vocabulary duplicate of channels/operations/messages); per-connection session identity — rejected (broadcast + client-side filtering at D9).

### D8: Event bus — producer/consumer decoupling via named events (event-driver)
A server-side event broker decouples REST producers from ws/SignalR consumers; the two models never reference each other.
- **Trigger — `x-event-trigger` on an OpenAPI response example** (list form): `{name, payload?, delay?, global?}`. Fired when that example is selected and its response produced. `delay` (ms) schedules delivery; `global: true` makes the event server-wide, otherwise it is schema-local.
- **Subscription — `x-send-events` on an AsyncAPI message example**: each entry is `{on: <eventName>, wait?: ms}` or a bare built-in (`receive`) / object built-in (`{on: connect, wait}` / `{on: cron, wait}`). When a named event fires (or the built-in trigger occurs), the subscribed message is emitted to the channel's consumers.
- **Delivery is broadcast; clients filter.** No session registry or account routing: every consumer of the channel receives the templated message, and consuming apps filter on event payload (`{$event.accountId}`). For a SignalR channel, emission targets its **open streams** as `StreamItem`s, or a server `Invocation` when no stream is open.
- **`{$event.*}` data source** — the event payload is exposed to consumer templates via a runtime `DataSource`, evaluated at emission time alongside `{$state.*}`/`{$env.*}`.
- **Management fire-event endpoint** fires a named event ad-hoc with the same delay/global semantics (covers monitoring and tests without a REST example).
- **Rationale**: keeps REST and AsyncAPI models orthogonal (the earlier `x-mock-push` cross-reference is removed); fan-out, cross-schema (`global`) delivery and ad-hoc firing all reuse the same broker + scheduler (delay → existing push scheduler, RS.AMG.1-4).
- **Alternative considered**: `x-mock-push` referencing a target channel/session from a REST example — rejected: couples the two models and reintroduces session routing; per-connection session identity — rejected: broadcast + `{$event.*}` filtering is simpler and matches real pub-sub.

### D9: Event-driven push into channels and SignalR streams
Events bridge producers to the ws/http delivery surface:
- A fired event resolves every message example subscribed to it (schema-local unless `global`), evaluates its templates with the event payload + schema state/env, and emits the resulting message to the channel's connected consumers (broadcast). `delay` on the trigger is honored before any emission; a consumer's `wait` applies to `connect`/`cron` built-ins.
- On a SignalR channel, emission targets **open streams** registered per `(connection, invocationId, channel)` — matching `RS.EVT.13`/`RS.SHR.18`; when none are open, the message is sent as a server `Invocation` (RS.SHR.19).
- **Rationale**: one broker serves spec-triggered, built-in-paced, and management-fired events; delivery logic (open stream vs server invocation vs raw ws) is owned by the protocol adapter, not the event bus.

### D10: Management API as the async-mocking control surface
`/_mock/examples` route resolution is extended to AsyncAPI route identifiers (protocol + address, plus method/action for http) so dynamic examples work for ws/http channels. A dedicated async-mocking surface is added for runtime consumer control:
- **Delayed push**: `delay` (ms) on push requests; delivery scheduled per-consumer; `0`/omitted = immediate.
- **Targeted/broadcast push**: optional `connectionId` selects one consumer; omitting broadcasts to all consumers of the channel.
- **Consumer discovery**: list active `connectionId`s per channel, including open SignalR streams.
- **Templated push payloads**: pushed payloads run through the existing runtime evaluator ({$state.*}, {$env.*}) using the schema namespace at delivery time.
- **Recurring push**: schedule a message at a fixed interval; cancellable by push ID.
- **Fire-event**: fire a named event ad-hoc with payload/delay/global, reusing the event broker (D8).
- **Connection lifecycle control**: force-disconnect a consumer by `connectionId` (with optional close reason/code) or simulate an abrupt drop (abort without a normal close frame) for behavior-testing reconnect/backoff logic.
- **Rationale**: keeps the server the single owner of connections/state; management API is the natural control plane (mirrors `/_mock/examples` philosophy).
- **Other useful async-mock extensions considered (future)**: correlation/reply simulation, consumer-group sequencing (e.g. RoundRobin on broadcast), batch push, rate-limited push, per-connection scripted push sessions.

### D11: Stream sequence/pacing via state + cron send-events (depth book deferred)
Streaming clients that need monotonic sequence numbers or cadence (the original Binance diff-depth story) are served by **existing primitives**, not a book engine:
- Counters/IDs live in the schema's state namespace (`x-mock-set-state`), referenced in payloads via `{$state.counter}` with increments.
- Pacing is emulated with the built-in **`cron` send-event** on a message example (D8) at the desired interval.
- No `x-mock-depth`, no `internal/depthbook`, no U/u book, no snapshot re-bootstrap in this change.
- **Rationale**: covers the generic "numbered, paced stream" pattern with zero new state machines; a real order-book engine (zero-qty deletes, snapshot↔stream `lastUpdateId` correlation, forced-reconnect re-bootstrap) is tightly coupled to one client and is deferred to a dedicated future change.
- **Alternative considered**: full `internal/depthbook` — rejected as out-of-scope for the MVP and too concrete to a single story.

## Risks / Trade-offs

- **benelser/go-asyncapi maturity** → Vendored and pinned via `replace`; `internal/asyncapi` abstraction keeps it out of the rest of the codebase; structural validation and `x-mock-*` extraction are done in `internal/asyncapi` so parser quirks don't leak. Swap path = new adapter + `replace` change.
- **SignalR wire complexity** → The overlay is ~a few hundred lines of protocol code; correctness is pinned by a spec-conformance integration test implementing our own frames (handshake → StreamInvocation → held-open stream → pushFill → cancel). Content stays declarative, so only framing/envelope logic is bespoke.
- **WebSocket upgrade vs reverse-proxy edge cases** (timeouts, ping/pong) → Use a battle-tested ws library (`github.com/gorilla/websocket`); keep default ping/pong grace periods.
- **Behavior drift between OpenAPI and AsyncAPI templating** → Reuse the exact same selection/state/history code paths (D3/D5); add parity integration tests over both spec kinds.
- **Startup failure surface grows** (unsupported protocol/version must fail fast) → Extend exit-code-3 tests; deterministic message listing unsupported protocol/version (now includes `amqp`).

## Migration Plan

- No breaking CLI changes: `--from`, `--prefix`, and config `schemas` are unchanged (AsyncAPI files are just accepted and auto-detected).
- Rolling back is not applicable mid-change; the change is additive except that `amqp` bindings switch from "accepted" (previously planned) to "unsupported". Since the change is unshipped, this is a design-level correction, not a migration.
- Docs updated in the same change: `docs/architecture.md` (loader/server sections), `docs/cli.md` (schema types), `docs/project.md` (structure additions for asyncapi loader/adapters/signalr overlay).

## Open Questions
- Whether the vendored `benelser/go-asyncapi` parser must later be replaced by a stricter 3.1.0 parser; the `internal/asyncapi` abstraction makes that a local change.
- Whether a future change should deliver the Binance diff-depth book engine (U/u continuity, zero-qty deletes, snapshot re-bootstrap) on top of the sequence/pacing primitives (D11).