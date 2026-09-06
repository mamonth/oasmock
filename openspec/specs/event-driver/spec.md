# event-driver Specification

## Purpose
Named event bus decoupling OpenAPI example triggers (x-event-trigger) from AsyncAPI message subscriptions (matched via `x-mock-match` against an event context), with {$event.*} payload templating and schema-local/global scoping.
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

### Requirement: Broadcast delivery with client-side filtering
Event-driven messages SHALL be broadcast to the consuming channel's connected consumers by default; consumers may filter by payload. When the example's `x-mock-match` additionally references `{$connection.*}`, delivery SHALL be narrowed to the consumers satisfying those per-connection conditions (two-phase recipient partition); the mock does not otherwise route by session or account.

#### Scenario RS.EVT.12: Broadcasting an event-driven message
- **WHEN** an event fires and a message example matching it exists on a channel with active consumers and no `{$connection.*}` conditions
- **THEN** the templated message is emitted to all consumers connected to that channel

#### Scenario RS.EVT.13: Emitting into an open SignalR stream
- **WHEN** an event fires and the matching channel is a SignalR hub stream with open invocation handles
- **THEN** the templated message is pushed as a `StreamItem` into the channel's open streams (per `signalr-hub-runtime`)

#### Scenario RS.EVT.14: Event with no subscribers
- **WHEN** a named event fires but no message example matches it
- **THEN** the event is accepted with no delivery (no error)

#### Scenario RS.EVT.15: Event with no consumers
- **WHEN** an event fires, a matching example exists, but the channel has no connected consumers
- **THEN** the event is accepted without error and no message is delivered

### Requirement: Event-driven emission on an AsyncAPI message example
The mock server SHALL emit an AsyncAPI message example in response to events when its `x-mock-match` references the event context. The event identity is matched via `{$event.name}` (named-event name or built-in kind `connect`/`receive`); payload conditions use `{$event.<field>}` and the whole payload via `{$event.data}`. The example's payload SHALL be templated at emission time against `{$event.*}`, `{$state.*}`, and `{$env.*}`. Periodic emission SHALL be declared with `x-mock-interval` rather than an event condition.

#### Scenario RS.EVT.22: Emitting on a named event
- **WHEN** a message example has `x-mock-match: {'{$event.name}': <eventName>}`
- **THEN** the message is emitted to the channel's consumers whenever that event fires

#### Scenario RS.EVT.23: Event payload in consumer template
- **WHEN** an event fires with a payload and the message example references `{$event.<key>}`
- **THEN** the expression resolves to the event payload value at emission time

#### Scenario RS.EVT.24: Built-in connect trigger
- **WHEN** a message example has `x-mock-match: {'{$event.name}': connect}`
- **THEN** the message is emitted to the (just-connected) consumer when it connects, subject to an optional `x-mock-delay`

#### Scenario RS.EVT.25: Built-in receive trigger
- **WHEN** a message example has `x-mock-match: {'{$event.name}': receive}` and the channel receives a client message
- **THEN** the message is emitted with the inbound client message exposed in the event context

#### Scenario RS.EVT.26: Periodic emission via x-mock-interval
- **WHEN** a message example declares `x-mock-interval: <ms>` instead of any event condition or match
- **THEN** the message is emitted repeatedly to the channel's consumers at the given interval until removed or the server shuts down
- **AND** the example SHALL NOT carry an `x-mock-match` (a periodically driven example has exactly one trigger); a spec declaring both is rejected at load

### Requirement: Per-connection event delivery
The mock server SHALL narrow event-driven delivery to consumers whose connection context satisfies the example's `{$connection.*}` conditions (e.g., `'{$connection.id}': '{$event.connectionId}'`), evaluating non-connection conditions once per emission and connection conditions per candidate.

#### Scenario RS.EVT.19: Targeted event delivery by connection id
- **WHEN** an example has `x-mock-match: {'{$event.name}': orderCreated, '{$connection.id}': '{$event.connectionId}'}` and the event fires with a `connectionId` payload
- **THEN** only the consumer whose connection id equals the payload value receives the message

### Requirement: x-send-events deprecation mapping
The mock server SHALL accept legacy `x-send-events` entries by mapping each `{on, wait}` to the unified form during loading, writing a deprecation note in verbose mode: `on` → `x-mock-match: {'{$event.name}': on}` for named/`connect`/`receive`, and `{on: cron, wait: N}` → `x-mock-interval: N`.

#### Scenario RS.EVT.18: Mapping legacy x-send-events to match
- **WHEN** a spec still uses `x-send-events: [{on: orderCreated}]` or `[{on: cron, wait: 1000}]`
- **THEN** the server behaves as if the example declared `x-mock-match: {'{$event.name}': orderCreated}` (respectively `x-mock-interval: 1000`) and logs a deprecation note in verbose mode
- **AND** a `{on: cron}` entry without a positive `wait` SHALL be rejected at load with an error naming the missing interval (an interval of 0 would otherwise silently register a dead reply example)

### Requirement: Management fire-event endpoint
The mock server SHALL expose a management API endpoint to fire a named event ad-hoc, reusing the event broker and its delay semantics.

#### Scenario RS.EVT.16: Firing an event via management API
- **WHEN** a management request fires a named event with a payload and optional delay
- **THEN** the server delivers it like a spec-triggered event (immediately or after the delay)

#### Scenario RS.EVT.17: Event payload templating at fire time
- **WHEN** an ad-hoc fired event payload contains `{$state.*}` or `{$env.*}` expressions
- **THEN** they are evaluated against the schema's isolated state namespace and environment before delivery

