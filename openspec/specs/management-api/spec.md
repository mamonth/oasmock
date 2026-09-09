## Purpose

HTTP management API for runtime control of the OASMock server, allowing dynamic addition of mock examples (OpenAPI and AsyncAPI targets with match/interval/delay), retrieval of request history, and a type-discriminated event resource to fire events.

## Requirements

### Requirement: Management API availability
The mock server SHALL provide an HTTP API for runtime management under the `/_mock` path prefix according to [openapi](../../../../openapi.yaml) spec

#### Scenario RS.MAPI.1: Accessing management endpoints
- **WHEN** the mock server is running
- **THEN** endpoints `/_mock/examples` and `/_mock/requests` are accessible

### Requirement: Add example endpoint
The mock server SHALL provide `POST /_mock/examples` to add a custom mock example.

#### Scenario RS.MAPI.2: Adding a simple example
- **WHEN** a POST request is sent to `/_mock/examples` with a valid `AddExampleRequest` JSON body
- **THEN** the server stores the example and responds with `AddExampleResponse` containing success and an example ID

#### Scenario RS.MAPI.3: Adding a conditional example
- **WHEN** the request includes `conditions` object with runtime expressions
- **THEN** the server stores the example and will match it only when conditions are satisfied

#### Scenario RS.MAPI.4: Adding a one-time example
- **WHEN** the request includes `once: true`
- **THEN** the server stores the example as one-time (disposed after first match)

#### Scenario RS.MAPI.5: Adding an example with validation disabled
- **WHEN** the request includes `validate: false`
- **THEN** the server does not validate the example data against the OpenAPI schema

#### Scenario RS.MAPI.34: Response body validation default
- **WHEN** `validate` is omitted (defaults to true) and the response body does not match the route's OpenAPI schema
- **THEN** the server responds with HTTP 400

#### Scenario RS.MAPI.6: Invalid request body
- **WHEN** the request body is missing required fields or malformed
- **THEN** the server responds with HTTP 400

### Requirement: Add example with TTL
The `AddExampleRequest` SHALL accept an optional `ttl` field (integer seconds).

#### Scenario RS.MAPI.16: Adding an example with TTL
- **WHEN** a POST request is sent to `/_mock/examples` with `ttl: 1`
- **THEN** the server accepts the request and stores the TTL value alongside the example

#### Scenario RS.MAPI.17: TTL field validation — negative value
- **WHEN** a POST request is sent to `/_mock/examples` with `ttl: -1`
- **THEN** the server responds with HTTP 400

#### Scenario RS.MAPI.18: TTL field is optional (omitted)
- **WHEN** a POST request is sent to `/_mock/examples` without a `ttl` field
- **THEN** the server accepts the request and the example has no expiration

### Requirement: Request history endpoint
The mock server SHALL provide `GET /_mock/requests` to retrieve request history.

#### Scenario RS.MAPI.7: Retrieving all requests
- **WHEN** a GET request is sent to `/_mock/requests` without query parameters
- **THEN** the server responds with up to 1000 most recent requests (default limit)

#### Scenario RS.MAPI.8: Filtering by path
- **WHEN** a `path` query parameter is provided
- **THEN** the server returns only requests matching that path

#### Scenario RS.MAPI.9: Filtering by method
- **WHEN** a `method` query parameter is provided (GET, POST, etc.)
- **THEN** the server returns only requests with that HTTP method

#### Scenario RS.MAPI.10: Pagination with limit and offset
- **WHEN** `limit` and `offset` query parameters are provided
- **THEN** the server returns at most `limit` requests starting from `offset`

#### Scenario RS.MAPI.11: Filtering by time range
- **WHEN** `time_from` and/or `time_till` query parameters are provided (milliseconds since epoch)
- **THEN** the server returns only requests within the specified time range

#### Scenario RS.MAPI.12: Limit exceeding maximum
- **WHEN** `limit` is greater than 1000
- **THEN** the server caps the limit to 1000

### Requirement: Request history data format
The request history SHALL include timestamp, URL, method, headers, and parsed body.

#### Scenario RS.MAPI.13: Request history item structure
- **WHEN** a request is processed by the mock server
- **THEN** an entry is added to history containing `ts`, `url`, `method`, `headers`, and `body` fields

### Requirement: Example response format
The `AddExampleRequest` and `AddExampleResponse` SHALL follow the schemas defined in openapi.yaml.

#### Scenario RS.MAPI.14: AddExampleRequest validation
- **WHEN** a request includes `path` (string) and `response` (ExampleResponse) fields
- **AND** optional fields `method`, `once`, `validate`, `conditions`
- **THEN** the request is considered valid

#### Scenario RS.MAPI.15: ExampleResponse structure
- **WHEN** an example response is provided
- **THEN** it includes `code` (integer), `headers` (object), and `body` (any JSON value)

### Requirement: Add example targeting an AsyncAPI route
The mock server SHALL accept a route identifier that resolves to an AsyncAPI channel/operation (protocol + address, and HTTP method/action when applicable) in `POST /_mock/examples`, with the same storage, matching, once/TTL, conditions, and validation semantics as OpenAPI routes. The AsyncAPI target vocabulary SHALL also cover channels served by a SignalR hub: an example may target a hub channel address (e.g. `/qoden/OpenOrders`) with an event/interval/condition trigger, and the server SHALL deliver into that hub's open streams when the trigger fires.

#### Scenario RS.MAPI.19: Adding a dynamic example for an AsyncAPI channel
- **WHEN** a POST request is sent to `/_mock/examples` with an AsyncAPI route identifier (protocol `ws`/`http` and a channel address)
- **THEN** the server stores the example for that channel and responds with `AddExampleResponse` containing success and an example ID

#### Scenario RS.MAPI.38: Adding a dynamic example for a SignalR hub channel
- **WHEN** a POST request is sent to `/_mock/examples` targeting a channel served by a SignalR hub (e.g. `/qoden/OpenOrders`) with an event/interval/condition trigger
- **THEN** the server resolves the target to the hub channel, registers the example, responds with success and an example ID, and delivers into the hub channel's open streams when the trigger fires (matching raw-ws delivery semantics)

#### Scenario RS.MAPI.20: Dynamic example used by AsyncAPI traffic
- **WHEN** a ws/http message arrives for an AsyncAPI channel that has a dynamic example with matching conditions
- **THEN** the server selects the dynamic example using the same selection pipeline as spec examples

#### Scenario RS.MAPI.21: No matching AsyncAPI route
- **WHEN** a POST request is sent to `/_mock/examples` with an AsyncAPI route identifier that does not match any loaded channel (raw ws/http or SignalR hub)
- **THEN** the server responds with HTTP 400 (no matching route)

### Requirement: SignalR hub consumers are addressable by path
For a hub served at a parameterized path (e.g. `x-signalr.path: /frontoffice/ws/account/{accountId}`), the management API consumer surface SHALL let a caller discover the concrete upgrade path (including path-parameter values such as a per-account `accountId`) of each SignalR connection, so a targeted async push can select the connections of one logical account on a shared hub channel.

#### Scenario RS.MAPI.39: Discovery exposes the hub path value
- **WHEN** a management request lists consumers of a hub channel and multiple SignalR consumers are connected over distinct per-account paths
- **THEN** each consumer record includes the path-parameter value identifying its account, enabling a targeted push to exactly one account's stream

### Requirement: Fire an event on the event bus
The management API SHALL expose `POST /_mock/events` to fire a named event ad-hoc. The request SHALL carry a required `name` identity (the `{$event.name}` matched by event-driven examples), along with optional `payload`, `delay`, and `global` fields, reusing the event broker and its delay semantics (per `event-driver`). The success response SHALL carry `success` and the fired event `name`. A request missing `name` SHALL be rejected with HTTP 400.

#### Scenario RS.MAPI.22: Firing an event via management API
- **WHEN** a `POST /_mock/events` request fires a named event with `name`, a payload, and an optional delay
- **THEN** the server delivers it like a spec-triggered event (immediately or after the delay) to matching event-driven message examples

#### Scenario RS.MAPI.23: Fire-event payload templating
- **WHEN** an ad-hoc fired event payload contains `{$state.*}` or `{$env.*}` expressions
- **THEN** they are evaluated against the schema's isolated state namespace and environment before delivery

#### Scenario RS.MAPI.32: Invalid event type
- **WHEN** a `POST /_mock/events` request omits the required `name` identity
- **THEN** the server responds with HTTP 400

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

### Requirement: Removing a dynamic example
The mock server SHALL provide `DELETE /_mock/examples/{exampleId}` to remove a dynamically added example and cancel any recurring delivery registered under that example ID.

#### Scenario RS.MAPI.30: Removing a dynamic example
- **WHEN** a DELETE request is sent to `/_mock/examples/{exampleId}` for an existing example
- **THEN** the server removes the example, stops any recurring delivery, and responds with success

#### Scenario RS.MAPI.31: Removing an unknown example
- **WHEN** a DELETE request is sent to `/_mock/examples/{unknownId}` that does not exist
- **THEN** the server responds with HTTP 404
