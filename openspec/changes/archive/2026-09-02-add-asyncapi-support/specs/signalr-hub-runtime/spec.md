# SignalR Hub Runtime

## Purpose

Serve an ASP.NET Core SignalR hub over the existing WebSocket transport so real SignalR clients (`@microsoft/signalr`, ASP.NET Core clients) can connect, invoke streaming and one-shot hub targets, and receive server-initiated pushes. The hub is declared at the document root via the `x-signalr` extension; streams map to AsyncAPI channels and one-shot invocations map to operations. Only the wire protocol (negotiate, handshake, `\x1e` framing, envelopes) is implemented by the mock.

## ADDED Requirements

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
- **THEN** the server responds 200 with `connectionToken`, `connectionId`, `negotiateVersion: 1`, and `availableTransports` listing WebSockets with Text and Binary transfer formats

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
The first message on a SignalR connection SHALL be the protocol handshake, and all subsequent messages SHALL be JSON terminated by the ASCII record separator `0x1E` (unit separator byte).

#### Scenario RS.SHR.14: Valid handshake
- **WHEN** the client's first WebSocket text frame is `{"protocol":"json","version":1}`
- **THEN** the server replies `{}\x1e` and switches to framed messaging

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