# extensions Delta

## Purpose

`x-mock-match` becomes the single selection extension across sync and async examples: it selects against an HTTP/request context for OpenAPI and against an event context (`{$event.*}`) for async-driven examples, filters recipients per connection (`{$connection.*}`), and is complemented by timing-only sibling extensions.

## ADDED Requirements

### Requirement: Event-context example matching
The mock server SHALL evaluate `x-mock-match` against an event context for async-driven examples. The event context SHALL expose the event identity as `{$event.name}` (the named-event name or the built-in kind), the whole event payload as `{$event.data}`, and each payload field as `{$event.<field>}`. `{$event.name}`/`{$event.data}` are reserved metadata names; payload fields with those names are shadowed.

#### Scenario RS.EXT.18: Matching the event identity
- **WHEN** an async-driven example has `x-mock-match: {'{$event.name}': orderCreated}` and the `orderCreated` event fires
- **THEN** the server emits the example to the channel's consumers

#### Scenario RS.EXT.19: Matching the event payload
- **WHEN** an async-driven example has `x-mock-match: {'{$event.accountId}': 'acc-1'}` (or a JSON-schema condition on `{$event.data}`)
- **AND** the fired event's payload satisfies the condition
- **THEN** the server emits the example; otherwise it does not

#### Scenario RS.EXT.21: Matching built-in events
- **WHEN** an async-driven example has `x-mock-match: {'{$event.name}': connect}` or `{'{$event.name}': receive}`
- **THEN** the example is emitted when a consumer connects to the channel, or when the channel receives a client message, respectively

### Requirement: Event-driven example classification
The mock server SHALL classify a spec example as event-driven when its `x-mock-match` references `{$event.*}`, as periodically driven when it declares `x-mock-interval`, and otherwise as a sync/async reply. An example SHALL be rejected at load when it mixes `{$event.*}` match conditions with `{$request.*}`/`{$message.*}`/`{$channel.*}` conditions, or when it declares both `x-mock-interval` and any `x-mock-match` conditions (a periodic emission is single-trigger and has no match context to honor). An event-driven example whose `{$event.name}` condition value is itself a runtime expression SHALL be rejected at load: the identity must be a literal string so the subscription key can always match a fired identity.

#### Scenario RS.EXT.20: Rejecting mixed match contexts
- **WHEN** a spec example's `x-mock-match` contains both `{$event.name}` and `{$message.payload.kind}` conditions in the same map
- **THEN** the server rejects the spec at load with a clear error

#### Scenario RS.EXT.28: Rejecting dual triggers
- **WHEN** a spec example declares both `x-mock-interval` and an `{$event.name}` match condition
- **THEN** the server rejects the spec at load with a clear error (any `x-mock-match` alongside `x-mock-interval`, event-driven or not, is a load error)

#### Scenario RS.EXT.33: Rejecting a non-literal event identity
- **WHEN** a spec example has `x-mock-match: {'{$event.name}': '{$state.envName}'}`
- **THEN** the server rejects the spec at load with a clear error instead of registering a subscription keyed by the literal expression string

#### Scenario RS.EXT.34: Wildcard identity without an {$event.name} pin
- **WHEN** an event-driven match references the event context only through condition values (no `{$event.name}` condition), e.g. `'{$connection.id}': '{$event.connectionId}'`
- **THEN** the example is registered as event-driven with a wildcard identity that evaluates against every fired event

### Requirement: Per-connection recipient matching
The mock server SHALL partition `x-mock-match` at delivery time: conditions referencing `{$connection.*}` on either side form the recipient filter, evaluated per candidate connection; all other conditions are evaluated once per emission. Only candidates that satisfy both phases receive the message. When no condition references `{$connection.*}`, the server SHALL broadcast to all consumers of the channel as today.

#### Scenario RS.EXT.24: Two-phase recipient partition
- **WHEN** an async-driven example has `x-mock-match` with a common condition and `'{$connection.id}': '{$event.connectionId}'`, and the event fires
- **THEN** the server evaluates the common condition once, then evaluates only the `{$connection.*}` condition against each candidate connection and delivers the payload to the connections whose id matches the event's `connectionId`

#### Scenario RS.EXT.25: Broadcast fast path
- **WHEN** an async-driven example has no `{$connection.*}` conditions and no `{$connection.*}` references
- **THEN** the server broadcasts the emitted message to all consumers of the channel (unchanged behavior, no per-connection evaluation)

#### Scenario RS.EXT.26: Single-recipient connect built-in
- **WHEN** a `connect` event fires and the connected consumer satisfies the example's `{$connection.*}` conditions
- **THEN** the server delivers the message to that single consumer only

#### Scenario RS.EXT.27: Connection context exposure
- **WHEN** a condition references `{$connection.id}`, `{$connection.channel}`, `{$connection.query.<key>}`, or `{$connection.header.<key>}`
- **THEN** the values resolve from the consumer's connection id, channel address, and metadata captured at upgrade

### Requirement: Timing extensions x-mock-interval and x-mock-delay
The mock server SHALL support `x-mock-interval` (positive integer milliseconds) on an async example to emit it repeatedly at that cadence, and `x-mock-delay` (integer milliseconds, default 0) to delay emission after an event fire. Neither is an event identity; `x-mock-interval` marks a periodically driven example. Timing values SHALL be integral milliseconds: a fractional value SHALL be rejected at load rather than silently truncated, and a periodically driven example SHALL honor `x-mock-skip` like every other example.

#### Scenario RS.EXT.22: Interval-driven periodic emission
- **WHEN** an async example declares `x-mock-interval: 1000`
- **THEN** the server emits the message to the channel's consumers roughly every 1000 ms until the example is removed or the server shuts down

#### Scenario RS.EXT.35: Rejecting a fractional x-mock-interval
- **WHEN** an async example declares `x-mock-interval: 2.5` (a fractional millisecond value)
- **THEN** the server rejects the spec at load with a clear error instead of truncating to 2 ms

#### Scenario RS.EXT.36: Rejecting a fractional x-mock-delay
- **WHEN** an async example declares `x-mock-delay: 2.5`
- **THEN** the server rejects the spec at load with a clear error instead of silently dropping the delay

#### Scenario RS.EXT.37: Skipping a periodically driven example
- **WHEN** a periodically driven example declares `x-mock-skip` and a consumer is connected to its channel
- **THEN** the server never emits the example's message while the skip flag is set

#### Scenario RS.EXT.23: Delayed event emission
- **WHEN** an async-driven example declares `x-mock-delay: 150` and its event fires
- **THEN** the server emits the message 150 ms after the fire

#### Scenario RS.EXT.30: Reply-path condition values stay literal
- **WHEN** an `x-mock-match` condition key references the reply context (`{$request.*}`/`{$message.*}`/`{$channel.*}`) and its value is a full runtime-expression string
- **THEN** the value is compared as a literal string, never pre-resolved (only conditions whose key references `{$event.*}` or `{$connection.*}` pre-resolve full-expression values)

### Requirement: Fail-closed match evaluation
An `x-mock-match` condition that references an expression source unavailable in the evaluation context SHALL fail closed (the example does not match) and, in verbose mode, SHALL log a warning.

#### Scenario RS.EXT.29: Event context unavailable in reply path
- **WHEN** a sync example references `{$event.*}` or a reply-path async example references `{$connection.*}`
- **THEN** the condition never matches and the server logs a verbose-mode warning rather than erroring