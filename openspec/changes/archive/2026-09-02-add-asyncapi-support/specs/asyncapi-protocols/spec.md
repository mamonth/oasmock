# AsyncAPI Protocols

## Purpose

Mapping AsyncAPI 3.x channels, operations, and messages onto runnable mock surfaces for the `http` and `ws` protocol bindings. `ws` channels are served as raw WebSockets, or as SignalR hub streams when the document declares a root-level `x-signalr` (see `signalr-hub-runtime`). This is an MVP: each protocol gets minimal but real serving so clients can connect and exchange mock messages. `amqp` (and any other protocol) is not served and fails startup with a clear error.

## ADDED Requirements

### Requirement: Channel to route mapping
The router SHALL convert AsyncAPI channels into mock routes according to their protocol binding. Channels without a recognized protocol binding, or operations referencing a channel with no bindings, SHALL be reported as unsupported.

#### Scenario RS.ASP.1: Mapping an HTTP channel
- **WHEN** an AsyncAPI channel declares an `http` binding with a method and path
- **THEN** the router creates a mock HTTP route matching that method and path

#### Scenario RS.ASP.2: Mapping a WebSocket channel
- **WHEN** an AsyncAPI channel declares a `ws` binding with an address (e.g., `wss://host/socket`)
- **THEN** the server exposes an upgradeable WebSocket endpoint at the corresponding relative path

#### Scenario RS.ASP.3: Declaring a SignalR hub document
- **WHEN** an AsyncAPI document declares a root-level `x-signalr` extension
- **THEN** the server serves the document's ws channels as a SignalR hub (negotiate + framed streams) per `signalr-hub-runtime`

#### Scenario RS.ASP.4: Channel with unknown protocol binding
- **WHEN** an AsyncAPI channel declares a protocol binding other than `http`, `ws`, or `amqp` (e.g., `kafka`)
- **THEN** the server fails to start and reports the unsupported protocol

#### Scenario RS.ASP.5: Channel without binding information
- **WHEN** an AsyncAPI channel has no `bindings` section usable to determine a server protocol
- **THEN** the router reports the channel as invalid with a clear error

### Requirement: Operation handling
The router SHALL map AsyncAPI send and receive operations onto concrete mock behaviors: send operations accept incoming messages and publish/produce the operation's reply message; receive operations emit the operation's message to connected clients.

#### Scenario RS.ASP.6: Send operation accepts messages
- **WHEN** a client sends a message to a channel whose operation `action` is `send`
- **THEN** the server accepts it and, when a reply message is present, responds with the reply message's example

#### Scenario RS.ASP.7: Receive operation emits messages
- **WHEN** a client connects to a channel whose operation `action` is `receive`
- **THEN** the server emits the operation's message example to the client (over ws) or exposes it for polling (over http)

### Requirement: Prefix handling for channels
Channel addresses SHALL honor the schema-level prefix used on the command line/config, while repeated-serving of a single channel is a non-goal for this MVP.

#### Scenario RS.ASP.8: Channel address with prefix
- **WHEN** an AsyncAPI schema is loaded with prefix `/v1` and declares channel `user/signedup`
- **THEN** the mock channel is served under the prefixed address `/v1/user/signedup`

### Requirement: Default responses
For a send operation without an explicit reply message, the server SHALL acknowledge the message using a protocol-appropriate default.

#### Scenario RS.ASP.9: Acknowledging a send with no reply
- **WHEN** a client sends a message over ws and the operation has no reply message
- **THEN** the server acknowledges receipt (ws: echo/ack frame) without producing a payload

#### Scenario RS.ASP.10: HTTP send without reply
- **WHEN** an HTTP POST arrives for a send operation that has no reply message
- **THEN** the server responds with HTTP 200 and an empty body