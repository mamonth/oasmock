## MODIFIED Requirements

### Requirement: Delayed example push to consumer
The mock server SHALL expose `POST /_mock/async/messages` (formerly `POST /_mock/async/push`) to deliver a message to consumers of an AsyncAPI channel, accepting an optional `delay` (milliseconds) before the push is delivered.

#### Scenario RS.AMG.1: Pushing with a delay
- **WHEN** a management request posts a message to `/_mock/async/messages` for an AsyncAPI channel with `delay: 500`
- **THEN** the message is delivered to the channel's connected consumers 500 ms after the request (and the push is accepted immediately)

#### Scenario RS.AMG.2: Pushing without a delay
- **WHEN** a management request posts a message to `/_mock/async/messages` without a `delay`
- **THEN** the message is delivered to connected consumers immediately

#### Scenario RS.AMG.3: Negative or zero delay validation
- **WHEN** a management request includes a negative `delay`
- **THEN** the server responds with HTTP 400; `delay: 0` is allowed and means immediate

#### Scenario RS.AMG.4: Pushing to a channel with no consumers
- **WHEN** a management request posts a message to a valid AsyncAPI channel that has no connected consumers
- **THEN** the server accepts the request without error and no message is delivered

### Requirement: Connection lifecycle control
The mock server SHALL allow a management request to terminate a connected consumer's connection via `DELETE /_mock/async/consumers/{connectionId}`, with optional close `reason`/`code` query parameters, or an `abrupt` flag to simulate an abrupt client-side drop.

#### Scenario RS.AMG.14: Force disconnecting a consumer
- **WHEN** a `DELETE /_mock/async/consumers/{connectionId}` request targets an active consumer without extra parameters
- **THEN** the server closes that consumer's connection with a normal close frame

#### Scenario RS.AMG.15: Disconnect with a close reason
- **WHEN** a management request force-disconnects a consumer via `DELETE /_mock/async/consumers/{connectionId}` with `code` and `reason` query parameters
- **THEN** the server closes the connection delivering that reason/code to the peer

#### Scenario RS.AMG.16: Disconnect of an unknown consumer
- **WHEN** a management request force-disconnects a `connectionId` that has no active connection
- **THEN** the server responds with HTTP 404

#### Scenario RS.AMG.17: Simulating an abrupt client drop
- **WHEN** a management request simulates a drop via `DELETE /_mock/async/consumers/{connectionId}` with an `abrupt=true` query parameter
- **THEN** the server aborts the connection without a normal close frame, mimicking a network-level loss