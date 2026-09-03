# event-driver Specification

## Purpose
Named event bus decoupling OpenAPI example triggers (x-event-trigger) from AsyncAPI message subscriptions (x-send-events), with {$event.*} payload templating and schema-local/global scoping.
## Requirements
### Requirement: Event trigger on an OpenAPI example
The mock server SHALL support firing named events from an OpenAPI response example via the `x-event-trigger` extension, triggered whenever that example is selected for a response. Multiple triggers per example SHALL be supported (list form).

#### Scenario RS.EVT.1: Firing a single event
- **WHEN** a selected OpenAPI example has `x-event-trigger` with a `name`
- **THEN** the server fires the named event with an empty payload after the response is produced

#### Scenario RS.EVT.2: Firing multiple events from one example
- **WHEN** an OpenAPI example has an `x-event-trigger` list with several `name` entries
- **THEN** the server fires each named event

#### Scenario RS.EVT.3: Event with payload
- **WHEN** an `x-event-trigger` entry includes `payload`
- **THEN** the event carries that payload, evaluable by consumers as `{$event.*}`

#### Scenario RS.EVT.4: Delayed event
- **WHEN** an `x-event-trigger` entry includes `delay`
- **THEN** the server delivers the event (and any resulting consumer messages) after that delay

### Requirement: Event name scoping
Event names SHALL be schema-local by default and server-wide only when declared `global: true`.

#### Scenario RS.EVT.5: Schema-local event
- **WHEN** an OpenAPI schema fires an event without `global: true`
- **THEN** only `x-send-events` subscriptions within the same schema receive it

#### Scenario RS.EVT.6: Global event
- **WHEN** an `x-event-trigger` entry sets `global: true`
- **THEN** the event is broadcast over all loaded schemas and any matching subscription anywhere receives it

### Requirement: Event subscription on an AsyncAPI message example
The mock server SHALL support subscribing an AsyncAPI message example to events via the `x-send-events` extension. Each entry references a named event or a built-in trigger (`receive`, `connect`, `cron`).

#### Scenario RS.EVT.7: Subscribing to a named event
- **WHEN** a message example has `x-send-events` containing `{on: <eventName>}`
- **THEN** the message is emitted to the channel's consumers whenever that event fires

#### Scenario RS.EVT.8: Event payload in consumer template
- **WHEN** an event fires with a payload and the subscribed message example references `{$event.<key>}`
- **THEN** the expression resolves to the event payload value at emission time

#### Scenario RS.EVT.9: Built-in connect trigger
- **WHEN** a message example's `x-send-events` contains `{on: connect}`
- **THEN** the message is emitted to a consumer when it connects (with an optional `wait` delay)

#### Scenario RS.EVT.10: Built-in cron trigger
- **WHEN** a message example's `x-send-events` contains `{on: cron, wait: <ms>}`
- **THEN** the message is emitted repeatedly to the channel's consumers at the given interval

#### Scenario RS.EVT.11: Built-in receive trigger
- **WHEN** a message example's `x-send-events` contains a flat `receive` entry
- **THEN** the message is emitted when the channel receives a matching client message

### Requirement: Broadcast delivery with client-side filtering
Event-driven messages SHALL be broadcast to the consuming channel's connected consumers; the mock does NOT route by session or account — consumers filter by payload.

#### Scenario RS.EVT.12: Broadcasting an event-driven message
- **WHEN** an event fires and a message example subscribed to it exists on a channel with active consumers
- **THEN** the templated message is emitted to all consumers connected to that channel

#### Scenario RS.EVT.13: Emitting into an open SignalR stream
- **WHEN** an event fires and the subscribed channel is a SignalR hub stream with open invocation handles
- **THEN** the templated message is pushed as a `StreamItem` into the channel's open streams (per `signalr-hub-runtime`)

#### Scenario RS.EVT.14: Event with no subscribers
- **WHEN** a named event fires but no message example subscribes to it
- **THEN** the event is accepted with no delivery (no error)

#### Scenario RS.EVT.15: Event with no consumers
- **WHEN** an event fires, a subscription exists, but the channel has no connected consumers
- **THEN** the event is accepted without error and no message is delivered

### Requirement: Management fire-event endpoint
The mock server SHALL expose a management API endpoint to fire a named event ad-hoc, reusing the event broker and its delay semantics.

#### Scenario RS.EVT.16: Firing an event via management API
- **WHEN** a management request fires a named event with a payload and optional delay
- **THEN** the server delivers it like a spec-triggered event (immediately or after the delay)

#### Scenario RS.EVT.17: Event payload templating at fire time
- **WHEN** an ad-hoc fired event payload contains `{$state.*}` or `{$env.*}` expressions
- **THEN** they are evaluated against the schema's isolated state namespace and environment before delivery

