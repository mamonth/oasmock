# event-driver Delta

## Purpose

Event-driven emission on AsyncAPI message examples moves from the `x-send-events` subscription key to `x-mock-match` evaluated against an event context; recurrence moves to the `x-mock-interval` timing extension and built-ins are actually fired.

## MODIFIED Requirements

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

## ADDED Requirements

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

## REMOVED Requirements

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

**Reason**: `x-send-events on:` duplicated `x-mock-match` (coarse event-name equality on top of the existing matcher) and was the only reject — event-driven emission now uses the unified matcher against an event context, and recurrence is the `x-mock-interval` timing extension rather than a `cron` event.

**Migration**: Replace `x-send-events: [{on: <name>}]` with `x-mock-match: {'{$event.name}': <name>}`; `{on: connect, wait: N}` with `x-mock-match: {'{$event.name}': connect}` + `x-mock-delay: N`; `{on: receive}` with `x-mock-match: {'{$event.name}': receive}`; `{on: cron, wait: N}` with `x-mock-interval: N`. Until the next release the old form keeps working through the loading shim.