# event-driver Delta

## Purpose

Event-driven emission on AsyncAPI message examples is expressed solely through `x-mock-match` against an event context and the `x-mock-interval` timing extension; the legacy `x-send-events` subscription key and its load-time mapping shim are removed after their one-release deprecation window.

## MODIFIED Requirements

### Requirement: Event name scoping
Event names SHALL be schema-local by default and server-wide only when declared `global: true`.

#### Scenario RS.EVT.5: Schema-local event
- **WHEN** an OpenAPI schema fires an event without `global: true`
- **THEN** only matching event-driven message examples within the same schema receive it

#### Scenario RS.EVT.6: Global event
- **WHEN** an `x-event-trigger` entry sets `global: true`
- **THEN** the event is broadcast over all loaded schemas and any matching subscription anywhere receives it

## REMOVED Requirements

### Requirement: x-send-events deprecation mapping
The mock server SHALL accept legacy `x-send-events` entries by mapping each `{on, wait}` to the unified form during loading, writing a deprecation note in verbose mode: `on` → `x-mock-match: {'{$event.name}': on}` for named/`connect`/`receive`, and `{on: cron, wait: N}` → `x-mock-interval: N`.

#### Scenario RS.EVT.18: Mapping legacy x-send-events to match
- **WHEN** a spec still uses `x-send-events: [{on: orderCreated}]` or `[{on: cron, wait: 1000}]`
- **THEN** the server behaves as if the example declared `x-mock-match: {'{$event.name}': orderCreated}` (respectively `x-mock-interval: 1000`) and logs a deprecation note in verbose mode
- **AND** a `{on: cron}` entry without a positive `wait` SHALL be rejected at load with an error naming the missing interval (an interval of 0 would otherwise silently register a dead reply example)

**Reason**: The one-release deprecation window has elapsed. The `x-send-events` subscription key duplicated `x-mock-match` (coarse `{$event.name}` equality over the unified matcher) and `{on: cron}` duplicated the `x-mock-interval` timing extension; the mapping shim kept a second subscription vocabulary alive inside the loader. Event-driven emission and recurrence are now fully expressed by `x-mock-match` and `x-mock-interval`.

**Migration**: Replace `x-send-events: [{on: <name>}]` with `x-mock-match: {'{$event.name}': <name>}`; `{on: connect, wait: N}` with `x-mock-match: {'{$event.name}': connect}` + `x-mock-delay: N`; `{on: receive}` with `x-mock-match: {'{$event.name}': receive}`; `{on: cron, wait: N}` with `x-mock-interval: N`. After removal, an `x-send-events` key is ignored: the message example is classified solely by its `x-mock-match`/`x-mock-interval`/reply trigger and loads without error.