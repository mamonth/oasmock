## MODIFIED Requirements

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

## ADDED Requirements

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
