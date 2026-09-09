## Context

See proposal.md — Why for the three confirmed defects. Current state that shapes the approach
(verified against the published `0.1.0` image and source at these locations):

- **Handshake** (`internal/server/signalr_conn.go` `runConnection`, `internal/server/signalr_protocol.go`
  `parseSignalRHandshake`): the first text frame is fed to `json.Unmarshal` verbatim. The project's own
  Go test client (`internal/server/signalr_integration_test.go`) sends the handshake *without* the record
  separator, so the parser's no-strip behaviour is never exercised. A real `@microsoft/signalr` client
  (`JsonHubProtocol.writeHandshakeRequest`) emits `{"protocol":"json","version":1}\x1e`, which fails
  `json.Unmarshal` with `invalid character '\x1e' after top-level value`; the connection is closed before
  any stream opens.
- **Push payload** (`internal/server/management_async.go` `asyncMessageRequest.Payload map[string]any`,
  wired through `handleAsyncMessage` → `evaluatePushPayload` → `pushToChannel`): the decode struct only
  accepts a JSON object. SignalR stream items and raw ws channel frames legitimately carry arrays (the
  Qoden `OpenOrders`/`BalanceUpdates` snapshots and diffs are arrays), so array payloads are rejected with
  HTTP 400. The OpenAPI contract mirrors the limitation (`MessagesRequest.payload: type: object`).
- **Hub-channel targeting** (`internal/server/server_management.go` `resolveExampleTarget` /
  `findAsyncRouteMapping`): the async branch matches only `s.mappings`, and x-signalr hub channels are
  deliberately **not** present there (`internal/loader/router.go` `buildAsyncRouteMappings` skips them under
  x-signalr; they are served by the hub, design D7). Registration of an example against `/qoden/OpenOrders`
  therefore returns `no matching route found`. Raw-ws `/_mock/async/messages` broadcast *does* reach hub
  streams via `hubMgr.SignalRPush` (`internal/server/hubmanager.go` `hubChannelForAddress`), so delivery
  plumbing exists — only the example-registration address resolution excludes hubs.
- **Per-account identity** (`internal/server/signalr_hub.go` `serveUpgrade`): a connection records
  `sc.query`/`sc.headers`, not the upgrade path; `hubDefaultChannel` is the same prefixed address for every
  connection of the hub, so `Candidates`/consumer discovery cannot distinguish two accounts sharing the hub.

## Goals / Non-Goals

**Goals:**
- Accept the spec-compliant record-separator-terminated SignalR handshake (and keep accepting the bare
  form, so the project's own Go tests keep passing without modification).
- Let `/_mock/async/messages`, `/_mock/examples`, and the event/interval delivery pipeline carry any JSON
  payload (objects and arrays) verbatim into hub streams and raw ws channels.
- Resolve example registrations against SignalR hub channels, and expose the per-account upgrade path in
  consumer metadata so a push can be scoped to one account's connection on a shared hub channel.

**Non-Goals:**
- Re-architecting the hub/registry split (design D7) — hub channels stay out of the raw-route mapping
  table; address resolution for hub channels is added where async example targeting needs it.
- Backwards-compat shims for the rejected payload shape: widening `payload` to any JSON value is strictly
  additive (object payloads keep working).
- Publishing the Docker image / SDK bump in this change — that is the follow-up release.

## Decisions

### D1: Strip the trailing record separator before parsing the handshake
Trim a single trailing `0x1E` from the first frame before `json.Unmarshal`, accepting both
`{"protocol":"json","version":1}` and `{"protocol":"json","version":1}\x1e`.

- **Why over "require the separator"**: the bare form is what the project's own Go integration tests send,
  and rejecting it would churn the whole test surface for no protocol benefit; the separator is a framing
  convenience the client may or may not include on the handshake frame.
- **Alternative considered**: parse the handshake with the same frame-splitting used for messages
  (`splitSignalRFrames`). Rejected — over-coupling; the handshake is a single JSON object, a trailing
  strip is sufficient and minimal.

### D2: Decode the management push payload as `any` (raw JSON), evaluate templates only on objects
Change `asyncMessageRequest.Payload` to `json.RawMessage` (or `any`), store it, and deliver the raw bytes
verbatim. Runtime-expression evaluation (`{$state.*}`, `{$env.*}`, per AMG.10) applies only when the
payload is a JSON object; arrays/scalars are delivered as-is.

- **Why**: expressions address object fields by path; evaluating a template over an array has no meaning.
  Passing arrays through untouched preserves the Qoden stream contract (array-typed `item`).
- **Alternative considered**: wrapping array payloads in a synthetic object (`{items: [...]}`) client-side.
  Rejected — it would change the wire shape the scheduler's handler receives and leak an implementation
  artifact into every consumer.

### D3: Resolve async example targets against hub channels too
`findAsyncRouteMapping` (or a new resolver used by `resolveExampleTarget`) consults both the raw-route
`mappings` table and the hub channels (`hubMgr.hubChannelForAddress`) when the target is an async
channel address. Once resolved to a hub channel, the registered example is delivered through the existing
event/interval pipeline, whose delivery path already reaches hub streams
(`message_delivery.go` `SignalRPush`).

- **Why**: reuse the existing delivery plumbing instead of adding a parallel one.
- **Alternative considered**: mapping hub channels into `s.mappings` as ws routes. Rejected — that would
  make the hub and the raw ws adapter compete for the same upgrade path (design D7 explicitly forbids it).

### D4: Capture the upgrade path on the SignalR connection and surface it in consumer metadata
In `serveUpgrade`, record the concrete request path (and chi path-param values, e.g. `accountId`) on the
`signalRConnection`. `Candidates`/`handleAsyncConsumers` include it in the consumer record; the SDK consumer
shape gains the field. Targeted push then matches on the captured value.

- **Why**: the scheduler's hub URL embeds the account in the path
  (`/frontoffice/ws/account/{accountId}`); without it no per-account targeting is possible. Query/headers
  do not carry the account.
- **Alternative considered**: forcing a per-account query param or header onto the connection.
  Rejected — the upstream `@microsoft/signalr` client does not send one, and the scheduler's hub URL is
  fixed to the path form.

## Risks / Trade-offs

- [Broadening `payload` to any JSON may let a malformed (non-JSON) body slip past the decoder] →
  Mitigation: keep the decoder strict about *syntax* (invalid JSON still 400); only the JSON *shape* is
  widened.
- [Exposing the upgrade path in consumer metadata changes the consumer record shape] →
  Mitigation: additive field; existing consumers of the list (connection id, channel, protocol, streams)
  are unaffected.
- [D3 touches example-registration address resolution, a shared code path] →
  Mitigation: hub resolution is attempted only after the raw-route scan misses; raw-ws/http behaviour is
  byte-for-byte unchanged.

## Migration Plan

- Land the source fix and the spec/docs updates in this change; release as `0.1.1` (or `0.2.0` given the
  additive API surface) and bump `oasmock-sdk` to match the widened consumer/payload types.
- No runtime migration: in-memory mock state, no persistence.

## Open Questions

None that change the specs or task breakdown. The exact chi route-parameter capture mechanism and the
consumer-record field name are implementation details left to the tasks.
