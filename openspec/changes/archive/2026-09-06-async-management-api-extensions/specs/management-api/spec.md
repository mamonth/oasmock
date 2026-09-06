# Management API Delta

## Purpose

Unified runtime example injection for sync (OpenAPI) and async (AsyncAPI) mocking: async targets take `match`/`interval`/`delay` mirroring the `x-mock-match`/`x-mock-interval`/`x-mock-delay` extensions, with strict single-trigger validation and a `type`-discriminated event resource.

## ADDED Requirements

### Requirement: Adding a runtime async-driven example
The `POST /_mock/examples` request SHALL accept, for AsyncAPI targets, an optional `match` object (mirroring `x-mock-match` against the event and connection contexts), an optional `interval` (positive integer ms for periodic emission), and an optional `delay` (integer ms). The mock server SHALL register the added message example (payload = `response.body`, headers = `response.headers`) as a live async-driven subscription delivered to the channel's consumers according to its match/interval, templating the payload at emission time against `{$event.*}`, `{$connection.*}`, `{$state.*}`, and `{$env.*}`.

#### Scenario RS.MAPI.24: Registering a named-event runtime example
- **WHEN** a POST request is sent to `/_mock/examples` with an AsyncAPI target, `response.body`, and `match: {'{$event.name}': orderCreated}`
- **THEN** the server registers the message as a live subscription, responds with success and an example ID, and delivers the message when the `orderCreated` event fires

#### Scenario RS.MAPI.25: Scheduling repeated delivery via interval
- **WHEN** a POST request includes `interval: 1000` for an AsyncAPI target
- **THEN** the message is delivered repeatedly at the 1000 ms interval until removed (or the server shuts down)

#### Scenario RS.MAPI.26: Subscribing to the connect and receive built-ins
- **WHEN** a POST request includes `match: {'{$event.name}': connect}` or `{'{$event.name}': receive}`
- **THEN** the message is delivered to a consumer when it connects to the channel, or when the channel receives a client message (with the inbound message payload available to templates), respectively

#### Scenario RS.MAPI.33: Targeting delivery by connection
- **WHEN** a POST request includes `match` with a `{$connection.*}` condition alongside an event condition
- **THEN** the registered message is delivered only to the channel's consumers satisfying that connection condition when the event fires

### Requirement: Strict example target validation
The `POST /_mock/examples` request SHALL reject field combinations that mix or misplace sync and async targeting with HTTP 400. An OpenAPI target requires `path` (and uses `response`); an AsyncAPI target requires `channel` (optionally `protocol`); `match`/`interval`/`delay` are only valid on AsyncAPI targets; a runtime example SHALL have exactly one trigger — `interval` OR an `{$event.*}`-based `match`, never both — and `interval` SHALL be a positive integer.

#### Scenario RS.MAPI.27: Mixing sync and async targeting
- **WHEN** a POST request includes both `path` and `channel`
- **THEN** the server responds with HTTP 400

#### Scenario RS.MAPI.28: match or interval on an OpenAPI target
- **WHEN** a POST request includes `path` with `match` (or `interval`) but no AsyncAPI target
- **THEN** the server responds with HTTP 400

#### Scenario RS.MAPI.29: Dual or invalid triggers
- **WHEN** a POST request includes both `interval` and an event-based `match`, or an `interval` that is not a positive integer
- **THEN** the server responds with HTTP 400

#### Scenario RS.MAPI.35: Non-event match on an async target
- **WHEN** a POST request includes an AsyncAPI target and a `match` whose conditions reference only `{$connection.*}` (or literal values) with no `{$event.*}` reference
- **THEN** the server responds with HTTP 400 and registers nothing (a runtime example needs a trigger; a connection-only match has none)

### Requirement: Removing a dynamic example
The mock server SHALL provide `DELETE /_mock/examples/{exampleId}` to remove a dynamically added example and cancel any recurring delivery registered under that example ID.

#### Scenario RS.MAPI.30: Removing a dynamic example
- **WHEN** a DELETE request is sent to `/_mock/examples/{exampleId}` for an existing example
- **THEN** the server removes the example, stops any recurring delivery, and responds with success

#### Scenario RS.MAPI.31: Removing an unknown example
- **WHEN** a DELETE request is sent to `/_mock/examples/{unknownId}` that does not exist
- **THEN** the server responds with HTTP 404

## MODIFIED Requirements

### Requirement: Fire an event on the event bus
The management API SHALL expose `POST /_mock/events` to fire a named event ad-hoc. The request SHALL carry a required `type` discriminator (`"fire"` for V1, extensible), along with `event`, `payload`, `delay`, and `global` fields, reusing the event broker and its delay semantics (per `event-driver`).

#### Scenario RS.MAPI.22: Firing an event via management API
- **WHEN** a `POST /_mock/events` request fires a named event with `type: fire`, a payload, and an optional delay
- **THEN** the server delivers it like a spec-triggered event (immediately or after the delay) to matching event-driven message examples

#### Scenario RS.MAPI.23: Fire-event payload templating
- **WHEN** an ad-hoc fired event payload contains `{$state.*}` or `{$env.*}` expressions
- **THEN** they are evaluated against the schema's isolated state namespace and environment before delivery

#### Scenario RS.MAPI.32: Invalid event type
- **WHEN** a `POST /_mock/events` request omits `type` or uses a type other than `fire`
- **THEN** the server responds with HTTP 400