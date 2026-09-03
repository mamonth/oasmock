# Management API Delta

## Purpose

The management API extends to AsyncAPI: dynamic example injection now resolves not only OpenAPI paths but also AsyncAPI channels/routes, so `/_mock/examples` works for ws/http channels served from AsyncAPI specs. The async-mocking surface (see `asyncapi-management`) additionally supports push by connection, consumer/stream discovery, recurring push, a fire-event endpoint, and connection lifecycle control.

## ADDED Requirements

### Requirement: Add example targeting an AsyncAPI route
The mock server SHALL accept a route identifier that resolves to an AsyncAPI channel/operation (protocol + address, and HTTP method/action when applicable) in `POST /_mock/examples`, with the same storage, matching, once/TTL, conditions, and validation semantics as OpenAPI routes.

#### Scenario RS.MAPI.19: Adding a dynamic example for an AsyncAPI channel
- **WHEN** a POST request is sent to `/_mock/examples` with an AsyncAPI route identifier (protocol `ws`/`http` and a channel address)
- **THEN** the server stores the example for that channel and responds with `AddExampleResponse` containing success and an example ID

#### Scenario RS.MAPI.20: Dynamic example used by AsyncAPI traffic
- **WHEN** a ws/http message arrives for an AsyncAPI channel that has a dynamic example with matching conditions
- **THEN** the server selects the dynamic example using the same selection pipeline as spec examples

#### Scenario RS.MAPI.21: No matching AsyncAPI route
- **WHEN** a POST request is sent to `/_mock/examples` with an AsyncAPI route identifier that does not match any loaded channel
- **THEN** the server responds with HTTP 400 (no matching route)

### Requirement: Fire an event on the event bus
The management API SHALL expose an endpoint to fire a named event ad-hoc, reusing the event broker and its delay semantics (per `event-driver`).

#### Scenario RS.MAPI.22: Firing an event via management API
- **WHEN** a management request fires a named event with a payload and optional delay
- **THEN** the server delivers it like a spec-triggered event (immediately or after the delay) to matching `x-send-events` consumers

#### Scenario RS.MAPI.23: Fire-event payload templating
- **WHEN** an ad-hoc fired event payload contains `{$state.*}` or `{$env.*}` expressions
- **THEN** they are evaluated against the schema's isolated state namespace and environment before delivery