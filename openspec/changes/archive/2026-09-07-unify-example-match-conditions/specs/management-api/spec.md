## MODIFIED Requirements

### Requirement: Adding a runtime async-driven example
The `POST /_mock/examples` request SHALL accept, for AsyncAPI targets, an optional `conditions` object (mirroring `x-mock-match` against the event and connection contexts), an optional `interval` (positive integer ms for periodic emission), and an optional `delay` (integer ms). The mock server SHALL register the added message example (payload = `response.body`, headers = `response.headers`) as a live async-driven subscription delivered to the channel's consumers according to its conditions/interval, templating the payload at emission time against `{$event.*}`, `{$connection.*}`, `{$state.*}`, and `{$env.*}`.

#### Scenario RS.MAPI.24: Registering a named-event runtime example
- **WHEN** a POST request is sent to `/_mock/examples` with an AsyncAPI target, `response.body`, and `conditions: {'{$event.name}': orderCreated}`
- **THEN** the server registers the message as a live subscription, responds with success and an example ID, and delivers the message when the `orderCreated` event fires

#### Scenario RS.MAPI.25: Scheduling repeated delivery via interval
- **WHEN** a POST request includes `interval: 1000` for an AsyncAPI target
- **THEN** the message is delivered repeatedly at the 1000 ms interval until removed (or the server shuts down)

#### Scenario RS.MAPI.26: Subscribing to the connect and receive built-ins
- **WHEN** a POST request includes `conditions: {'{$event.name}': connect}` or `{'{$event.name}': receive}`
- **THEN** the message is delivered to a consumer when it connects to the channel, or when the channel receives a client message (with the inbound message payload available to templates), respectively

#### Scenario RS.MAPI.33: Targeting delivery by connection
- **WHEN** a POST request includes `conditions` with a `{$connection.*}` condition alongside an event condition
- **THEN** the registered message is delivered only to the channel's consumers satisfying that connection condition when the event fires

#### Scenario RS.MAPI.36: Async conditions are honored
- **WHEN** a POST request targets an AsyncAPI channel and carries `conditions` referencing the reply context (`{$request.*}`/`{$message.*}`/`{$channel.*}`)
- **THEN** the example is selected by the same reply-context selection pipeline as spec examples (async `conditions` are honored, not silently ignored)

### Requirement: Strict example target validation
The `POST /_mock/examples` request SHALL reject field combinations that mix or misplace sync and async targeting with HTTP 400. An OpenAPI target requires `path` (and uses `response`); an AsyncAPI target requires `channel` (optionally `protocol`); `conditions`/`interval`/`delay` are validated per target — `interval` and `delay` are only valid on AsyncAPI targets; a runtime example SHALL have exactly one trigger — `interval` OR an `{$event.*}`-based `conditions`, never both — and `interval` SHALL be a positive integer. The request schema SHALL NOT contain a `match` field.

#### Scenario RS.MAPI.27: Mixing sync and async targeting
- **WHEN** a POST request includes both `path` and `channel`
- **THEN** the server responds with HTTP 400

#### Scenario RS.MAPI.28: match or interval on an OpenAPI target
- **WHEN** a POST request includes `path` with `interval` (or `delay`) but no AsyncAPI target
- **THEN** the server responds with HTTP 400

#### Scenario RS.MAPI.29: Dual or invalid triggers
- **WHEN** a POST request includes both `interval` and an event-based `conditions`, or an `interval` that is not a positive integer
- **THEN** the server responds with HTTP 400

#### Scenario RS.MAPI.35: Non-event match on an async target
- **WHEN** a POST request includes an AsyncAPI target and `conditions` referencing only `{$connection.*}` (or literal values) with no `{$event.*}` reference
- **THEN** the server responds with HTTP 400 and registers nothing (a runtime example needs a trigger; a connection-only match has none)

#### Scenario RS.MAPI.37: Legacy match field rejected
- **WHEN** a POST request to `/_mock/examples` includes a `match` field
- **THEN** the server responds with HTTP 400 and registers nothing (the `match` field is removed; use `conditions`)
