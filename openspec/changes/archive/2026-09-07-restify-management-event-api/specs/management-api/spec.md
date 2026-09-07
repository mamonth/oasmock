## MODIFIED Requirements

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