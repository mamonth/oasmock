# asyncapi-management Specification

## Purpose
Management API endpoints for driving AsyncAPI mocking: delayed/targeted/broadcast push, consumer discovery, recurring schedules, fire-event, and connection lifecycle control.
## Requirements
### Requirement: Delayed example push to consumer
The mock server SHALL extend the management API with an endpoint to push a message example to consumers of an AsyncAPI channel, accepting an optional `delay` (milliseconds) before the push is delivered.

#### Scenario RS.AMG.1: Pushing with a delay
- **WHEN** a management request pushes a message to an AsyncAPI channel with `delay: 500`
- **THEN** the message is delivered to the channel's connected consumers 500 ms after the request (and the push is accepted immediately)

#### Scenario RS.AMG.2: Pushing without a delay
- **WHEN** a management request pushes a message without a `delay`
- **THEN** the message is delivered to connected consumers immediately

#### Scenario RS.AMG.3: Negative or zero delay validation
- **WHEN** a management request includes a negative `delay`
- **THEN** the server responds with HTTP 400; `delay: 0` is allowed and means immediate

#### Scenario RS.AMG.4: Pushing to a channel with no consumers
- **WHEN** a management request pushes a message to a valid AsyncAPI channel that has no connected consumers
- **THEN** the server accepts the request without error and no message is delivered

### Requirement: Targeted and broadcast push
The pushed message SHALL be deliverable to a single consumer connection or broadcast to all consumers of the channel.

#### Scenario RS.AMG.5: Pushing to a specific consumer
- **WHEN** a management request includes a `connectionId` for an active consumer
- **THEN** only that consumer receives the message

#### Scenario RS.AMG.6: Broadcasting to all consumers
- **WHEN** a management request omits `connectionId`
- **THEN** all consumers currently connected to the channel receive the message

#### Scenario RS.AMG.7: Unknown consumer reference
- **WHEN** a management request includes a `connectionId` that has no active connection
- **THEN** the server responds with HTTP 404

### Requirement: Connected consumer discovery
The mock server SHALL expose the currently connected consumers per AsyncAPI channel, including open SignalR streams.

#### Scenario RS.AMG.8: Listing connected consumers
- **WHEN** a management request queries consumers for an AsyncAPI channel with active connections
- **THEN** the server returns the consumer list with connection IDs, channel/address details, and open streams (for SignalR hubs)

#### Scenario RS.AMG.9: Listing consumers for a channel with no connections
- **WHEN** a management request queries consumers for an AsyncAPI channel with no active connections
- **THEN** the server returns an empty list

### Requirement: Templated push payloads
Pushed message payloads SHALL support runtime expressions ({$state.*}, {$env.*}) evaluated at delivery time, using the schema's state namespace.

#### Scenario RS.AMG.10: Pushing a templated payload
- **WHEN** a management request pushes a payload containing `{$state.counter}` or `{$env.X}`
- **THEN** the expression is evaluated against the schema's state/ environment before delivery

#### Scenario RS.AMG.11: Invalid expression in pushed payload
- **WHEN** a management request pushes a payload containing an unresolvable or malformed expression
- **THEN** the server rejects the request with HTTP 400

### Requirement: Recurring scheduled push
The mock server SHALL support scheduling repeated pushes of a message example to a channel at a fixed interval.

#### Scenario RS.AMG.12: Scheduling a recurring push
- **WHEN** a management request schedules a push with an `interval` in milliseconds
- **THEN** the message is delivered repeatedly at that interval until stopped (or the server shuts down)

#### Scenario RS.AMG.13: Stopping a recurring push
- **WHEN** a management request stops a previously scheduled recurring push (by its push ID)
- **THEN** no further deliveries occur for that schedule

### Requirement: Connection lifecycle control
The mock server SHALL allow a management request to terminate a connected consumer's connection, with an optional close reason, or to simulate an abrupt client-side drop.

#### Scenario RS.AMG.14: Force disconnecting a consumer
- **WHEN** a management request force-disconnects a consumer by `connectionId`
- **THEN** the server closes that consumer's connection with a normal close frame

#### Scenario RS.AMG.15: Disconnect with a close reason
- **WHEN** a management request force-disconnects a consumer including a close reason/code
- **THEN** the server closes the connection delivering that reason/code to the peer

#### Scenario RS.AMG.16: Disconnect of an unknown consumer
- **WHEN** a management request force-disconnects a `connectionId` that has no active connection
- **THEN** the server responds with HTTP 404

#### Scenario RS.AMG.17: Simulating an abrupt client drop
- **WHEN** a management request simulates a drop for a consumer
- **THEN** the server aborts the connection without a normal close frame, mimicking a network-level loss

### Requirement: Fire an event on the event bus
The mock server SHALL expose a management endpoint to fire a named event ad-hoc, reusing the event broker and its delay semantics (per `event-driver`).

#### Scenario RS.AMG.20: Firing an event via management API
- **WHEN** a management request fires a named event with a payload and optional delay
- **THEN** the server delivers it like a spec-triggered event (immediately or after the delay)

#### Scenario RS.AMG.21: Fire-event payload templating
- **WHEN** an ad-hoc fired event payload contains `{$state.*}` or `{$env.*}` expressions
- **THEN** they are evaluated against the schema's isolated state namespace and environment before delivery

