# signalr-hub-runtime Specification

## Purpose
ASP.NET Core SignalR-compatible hub serving for AsyncAPI documents declaring root x-signalr: negotiate, token-correlated upgrade, handshake, \x1e framing, streams, invocations and server pushes.
## Requirements
### Requirement: Root-level x-signalr extension
The mock server SHALL treat a parseable AsyncAPI document whose root declares `x-signalr` as a single SignalR hub, served over the document's WebSocket channels.

#### Scenario RS.SHR.1: Declaring a SignalR hub document
- **WHEN** an AsyncAPI document has a root-level `x-signalr` extension with a hub path
- **THEN** the server serves the document as one SignalR hub at that path, exposing a negotiate endpoint and framed WebSocket streams

#### Scenario RS.SHR.2: One hub per document
- **WHEN** an AsyncAPI document declares `x-signalr` with a single hub configuration
- **THEN** the server registers exactly one hub for that document

### Requirement: Streams map to channels
WebSocket channels in an `x-signalr` document SHALL be streamable hub targets: a client `StreamInvocation` whose `target` is a channel ID is answered by the channel's snapshot message, which stays open for further items.

#### Scenario RS.SHR.3: StreamInvocation by channel ID
- **WHEN** a client sends a `StreamInvocation` (type 4) with `target` equal to a declared channel ID
- **THEN** the server emits the channel's snapshot example as a `StreamItem` (type 2) on the client's `invocationId`

#### Scenario RS.SHR.4: Stream held open
- **WHEN** the snapshot `StreamItem` has been sent
- **THEN** the server does NOT send a `Completion`; the stream stays open and registered for that `(connection, invocationId)`

#### Scenario RS.SHR.5: Unknown channel target
- **WHEN** a `StreamInvocation` names a `target` that matches no channel ID
- **THEN** the server replies with a `Completion` (type 3) carrying an error for that invocation

### Requirement: One-shot invocations map to operations
Operations in an `x-signalr` document SHALL be invocable as one-shot hub targets: a client `Invocation` (type 1) whose `target` is an operation ID is answered by a `Completion` with the operation's message example.

#### Scenario RS.SHR.6: Invocation by operation ID
- **WHEN** a client sends an `Invocation` with `target` equal to an operation ID
- **THEN** the server replies with a `Completion` (type 3) carrying the operation's message example as the result

#### Scenario RS.SHR.7: Unknown operation target
- **WHEN** an `Invocation` names a target matching no operation ID
- **THEN** the server replies with a `Completion` carrying an error for that invocation

### Requirement: Negotiate endpoint
For the hub path, the server SHALL expose `POST {hubPath}/negotiate` returning supported transport info and a connection token used by the client's subsequent WebSocket upgrade.

#### Scenario RS.SHR.8: Successful negotiation
- **WHEN** a client POSTs to `{hubPath}/negotiate` with `negotiateVersion=1`
- **THEN** the server responds 200 with `connectionToken`, `connectionId`, `negotiateVersion: 1`, and `availableTransports` listing WebSockets with the Text transfer format (Binary is not offered because the handshake rejects binary frames, RS.SHR.15)

#### Scenario RS.SHR.9: Negotiate protocol version
- **WHEN** a client requests negotiation without `negotiateVersion` (treated as 0)
- **THEN** the server responds with `negotiateVersion: 1` (its supported version) and includes both `connectionToken` and `connectionId`

#### Scenario RS.SHR.10: Negotiate for an unsupported transport
- **WHEN** a client requests a transport other than WebSockets (e.g., server-sent events or long polling)
- **THEN** the server lists WebSockets only; upgrades for SSE/long-polling return HTTP 400

### Requirement: WebSocket upgrade with token correlation
The server SHALL require the WebSocket upgrade request to the hub path to carry the `id` query parameter matching a previously issued connection token.

#### Scenario RS.SHR.11: Upgrade with matching token
- **WHEN** a client upgrades to the hub path with `?id=<token>` where the token was issued by negotiate
- **THEN** the server accepts the upgrade and binds the connection to that token/connection

#### Scenario RS.SHR.12: Upgrade with unknown token
- **WHEN** a client upgrades with an `id` token that was not issued
- **THEN** the server rejects the upgrade with HTTP 404

#### Scenario RS.SHR.13: Upgrade without token
- **WHEN** a client upgrades without an `id` parameter
- **THEN** the server binds the connection to a fresh internally generated token so the connection can still be addressed by `connectionId`

### Requirement: Handshake and framing
The first message on a SignalR connection SHALL be the protocol handshake, and all subsequent messages SHALL be JSON terminated by the ASCII record separator `0x1E` (unit separator byte). The handshake request itself uses the same framing: a spec-compliant client (`@microsoft/signalr`) terminates the handshake JSON with the record separator, and the server SHALL accept the handshake with or without that trailing separator.

#### Scenario RS.SHR.14: Valid handshake
- **WHEN** the client's first WebSocket text frame is `{"protocol":"json","version":1}`
- **THEN** the server replies `{}\x1e` and switches to framed messaging

#### Scenario RS.SHR.23: Handshake terminated by the record separator
- **WHEN** the client's first WebSocket text frame is `{"protocol":"json","version":1}\x1e` (the framing the `@microsoft/signalr` client emits)
- **THEN** the server strips the trailing `0x1E`, parses the handshake successfully, replies `{}\x1e`, and switches to framed messaging (a record-separator-terminated handshake is NOT rejected as malformed JSON)

#### Scenario RS.SHR.15: Unsupported protocol handshake
- **WHEN** the client's first frame requests a protocol other than `json` (e.g., `messagepack`)
- **THEN** the server sends a handshake error and closes the connection

#### Scenario RS.SHR.16: Framed messages carry the record separator
- **WHEN** the server sends an `Invocation`, `StreamItem`, or `Completion`
- **THEN** the message JSON is terminated by the `0x1E` byte, and multiple messages may share one WebSocket text frame separated by that byte

### Requirement: Streaming invocation lifecycle
A `StreamInvocation` to a channel target SHALL produce a snapshot, keep the stream open for further items, and complete on `CancelInvocation` or stream end.

#### Scenario RS.SHR.17: Cancel closes the stream
- **WHEN** the client sends a `CancelInvocation` (type 5) for an open `invocationId`
- **THEN** the server sends a `Completion` (type 3) and removes the stream from the open-stream registry

#### Scenario RS.SHR.18: Event-driven item appended to open stream
- **WHEN** a server-initiated event triggers a message on a channel with open stream handles
- **THEN** the server emits the templated message as an additional `StreamItem` on each open `invocationId` without completing the stream (per `event-driver` RS.EVT.13)

### Requirement: Server-initiated one-shot push
For a SignalR hub, a server-side push that does not target an open stream SHALL be sent as a server-to-client `Invocation` with a server-assigned invocation id.

#### Scenario RS.SHR.19: Server Invocation push
- **WHEN** an event-driven message is emitted for a hub channel but no open stream matches
- **THEN** the server sends an `Invocation` (type 1) with `invocationId: <server-id>` and the message as `arguments`

### Requirement: Ping handling
The server SHALL respond to SignalR `Ping` messages (type 6); pings carry no invocation id.

#### Scenario RS.SHR.20: Ping is echoed
- **WHEN** the client sends `{type:6}`
- **THEN** the server replies `{type:6}` without affecting any streams

### Requirement: Open stream registry
The server SHALL keep an open-stream registry per connection so event-driven messages can be pushed into held-open streams and so management discovery can list them.

#### Scenario RS.SHR.21: Registry tracks open streams
- **WHEN** one or more streams are open on a connection
- **THEN** the registry retains `(connectionId, invocationId, channel ID)` so discovery and push endpoints can list and target them

#### Scenario RS.SHR.22: Delivery deduplicates per connection
- **WHEN** a connection has N open streams on a channel and a payload is delivered
- **THEN** the payload reaches that connection's N streams exactly once each (never N×N): delivery candidates are unique per connection even when several streams are open

### Requirement: Stream items carry arbitrary JSON payloads
A `StreamItem` emitted for a channel stream (the snapshot on `StreamInvocation`, or a later server push) SHALL carry the channel message payload verbatim, whatever JSON shape it has — an object, an array, or a scalar. Neither the hub framing nor the server push path SHALL constrain the payload to JSON objects; array payloads (e.g. an order-list diff `[{orderId: "grid-1"}, ...]`) must pass through unchanged as the `item` of a `StreamItem`.

#### Scenario RS.SHR.24: Pushing an array payload to an open stream
- **WHEN** a server push delivers an array payload to a channel that has an open stream
- **THEN** the consumer receives a `StreamItem` (type 2) whose `item` is exactly that JSON array, and the stream stays open

#### Scenario RS.SHR.25: Array snapshot on stream open
- **WHEN** a client opens a stream whose channel message payload is a JSON array
- **THEN** the snapshot `StreamItem` carries the array verbatim as `item`

### Requirement: Hub connections expose the per-account path
For a hub served at a parameterized path (e.g. `x-signalr.path: /frontoffice/ws/account/{accountId}`), each connection SHALL retain the concrete upgrade path it arrived on, including the matched path-parameter values, and expose them as connection metadata. Consumer discovery and per-connection recipient matching SHALL be able to address a connection by the value captured from its hub path, so a push can be scoped to one logical account's connections on a shared hub channel rather than only by query parameter or header.

#### Scenario RS.SHR.26: Per-account connections are distinguishable
- **WHEN** two clients open streams on the same hub channel over different per-account paths (e.g. `/frontoffice/ws/account/qa-A` and `/frontoffice/ws/account/qa-B`)
- **THEN** each connection's metadata exposes its captured path value (its `accountId`), and a targeted push can be delivered to exactly one account's stream without reaching the other

### Requirement: Event/interval examples can target hub channels
An async message example registered through the management API (event-driven, interval-driven, or condition-matched) SHALL be able to target a channel served by a SignalR hub and deliver into that hub's open streams, matching the delivery semantics raw `ws` channels already enjoy.

#### Scenario RS.SHR.27: Registering an event example for a hub channel
- **WHEN** a management API registration targets a SignalR hub channel address (e.g. `/qoden/OpenOrders`) with an event or interval trigger
- **THEN** the server registers the example and delivers it to the hub channel's open streams when the trigger fires

#### Scenario RS.SHR.28: Unsupported hub channel target is rejected
- **WHEN** a management API registration targets an address that matches no hub channel and no raw ws/http channel
- **THEN** the server responds with an error and registers nothing

