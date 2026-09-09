## Why

The mock's SignalR surface is not interoperable with real `@microsoft/signalr` clients, and the
management API cannot drive the two data shapes those clients actually consume. Three defects were
confirmed empirically against the published `0.1.0` image (pinned by downstream consumers such as
`xrocket-monorepo`'s mm-qoden integration suite):

1. The hub rejects the standard client handshake because it does not strip the record separator.
2. The push API accepts only JSON **objects**, but SignalR hub streams and raw ws channels legitimately
   carry **arrays** as the stream item (e.g. an order-list diff).
3. Event/interval-driven async examples cannot target a SignalR hub channel, and hub consumers carry no
   per-account identity, so a stream cannot be addressed per logical account.

## What Changes

- **Handshake framing (`signalr-hub-runtime`)** — the hub SHALL accept a spec-compliant
  `@microsoft/signalr` first frame whose handshake JSON is terminated by the ASCII record separator
  (`0x1E`), in addition to the bare handshake the project's own Go test client sends. The current
  implementation `json.Unmarshal`s the first frame verbatim, so the trailing `0x1E` from a real client
  fails with `invalid handshake: invalid character '\x1e' after top-level value` and the connection is
  dropped before any stream opens. RS.SHR.14 is corrected to reflect that the handshake request uses the
  same framing as messages.

- **Push payload shape (`asyncapi-management`, `management-api`)** — `POST /_mock/async/messages`
  (and the SDK `pushToChannel`) SHALL accept any JSON value — object **or array** — as the pushed
  payload, delivering it verbatim as the channel message / SignalR stream `item`. Today the request
  decoder types the payload as `map[string]any`, so an array body is rejected with
  `json: cannot unmarshal array into Go struct field ... of type map[string]interface {}`. The OpenAPI
  contract (`MessagesRequest.payload`) and the Go decode struct are updated together.

- **Targeting a SignalR hub channel from the management API (`management-api`, `signalr-hub-runtime`)**
  — an async example registration (`POST /_mock/examples`) SHALL accept a SignalR hub channel address and
  drive its open streams (event/interval/conditions), not only raw `ws`/`http` channels. Today
  `resolveExampleTarget`/`findAsyncRouteMapping` scan only raw-route mappings, and x-signalr hub channels
  are deliberately not mapped to raw ws routes, so registration fails with `no matching route found`.
  Hub consumers additionally SHALL expose the upgrade path (including the per-account `{accountId}`
  segment when the hub path is parameterized) in their connection metadata so a push can be scoped to one
  account's stream instead of broadcasting to every open stream on the channel.

## Capabilities

### New Capabilities

None. This change repairs existing behavior; no new capability domain is introduced.

### Modified Capabilities

- `signalr-hub-runtime`: handshake framing accepts the spec-compliant record-separator-terminated
  handshake (RS.SHR.14); server push and snapshot payloads carry any JSON value; hub connection metadata
  exposes the upgrade path for per-account addressing.
- `asyncapi-management`: `POST /_mock/async/messages` accepts object and array payloads and delivers
  them verbatim; consumer discovery exposes the upgrade path.
- `management-api`: `/_mock/examples` async targeting resolves SignalR hub channels; the `MessagesRequest`
  payload schema and `protocol` vocabulary cover hub targets.

## Impact

- **API**: `api/openapi.yaml` — widen `MessagesRequest.payload` to any JSON value; add SignalR hub
  channel addressing/protocol vocabulary to the async example target.
- **Handlers**: `internal/server/server_management.go` (`resolveExampleTarget`, `findAsyncRouteMapping`,
  `handleAddExample`), `internal/server/management_async.go` (`asyncMessageRequest.Payload` decode,
  consumer discovery response).
- **SignalR**: `internal/server/signalr_conn.go` (handshake frame parse), `internal/server/signalr_hub.go`
  (per-connection path capture), `internal/server/hubmanager.go` (Candidates channel/address metadata),
  `internal/server/signalr_registry.go` (stream delivery of arbitrary payloads).
- **SDK**: `oasmock-sdk` `pushToChannel`/`getConsumerList`/`onRequest` — typing already accepts
  `unknown` payloads, so mostly unaffected, but the pushed `payload` type and consumer shape follow the
  widened contract.
- **Downstream**: consumers pinned to `itmamonth/oasmock:0.1.0` (e.g. the xrocket mm-qoden integration
  suite) unblocked once a fixed image is published; this change documents the required behavior.
