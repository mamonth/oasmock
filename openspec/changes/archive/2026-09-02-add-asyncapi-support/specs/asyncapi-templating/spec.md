# AsyncAPI Templating

## Purpose

Bring the complete OpenAPI templating realization to AsyncAPI message examples: runtime expression evaluation (`{$...}`), `x-mock-*` extension-driven example selection and response shaping, and state/history integration — identical behavior to what OpenAPI operations enjoy today. This includes the event-driven data source `{$event.*}` for messages emitted by the `event-driver` bus.

## ADDED Requirements

### Requirement: Event data source in message templates
The mock server SHALL expose event payload data to subscribed AsyncAPI message examples as the `{$event.*}` data source, evaluated at emission time.

#### Scenario RS.ATM.17: Evaluating an event payload expression
- **WHEN** a message example subscribed to an event (per `event-driver` RS.EVT.7) references `{$event.orderId}`
- **AND** the fired event's payload contains key `orderId`
- **THEN** the expression evaluates to that event payload value at emission time

#### Scenario RS.ATM.18: Sequence/pacing via state and cron trigger
- **WHEN** a mock consumer needs monotonic sequence numbers or paced delivery for a stream
- **THEN** the server supports it with `x-mock-set-state` counters referenced via `{$state.*}` and the built-in `cron` send-event trigger (per `event-driver` RS.EVT.10); no dedicated sequence engine is required

### Requirement: Runtime expression evaluation on message examples
The mock server SHALL evaluate runtime expressions in AsyncAPI message examples using the same evaluator used for OpenAPI, exposing protocol-relevant data sources: the incoming message payload, headers, the channel address, server state, environment, and event payloads.

#### Scenario RS.ATM.1: Evaluating payload expression
- **WHEN** a message example contains expression `{$message.payload.id}`
- **AND** the client message payload contains property `id`
- **THEN** the expression evaluates to that property value

#### Scenario RS.ATM.2: Evaluating header expression
- **WHEN** a message example contains expression `{$message.header.trace}`
- **AND** the client message carries header `trace`
- **THEN** the expression evaluates to the header value

#### Scenario RS.ATM.3: Evaluating channel parameter expression
- **WHEN** a message example contains expression `{$channel.sid}`
- **AND** the channel defines parameter `sid` captured from the address
- **THEN** the expression evaluates to the captured parameter value

#### Scenario RS.ATM.4: Evaluating state expression
- **WHEN** a message example contains expression `{$state.counter}`
- **AND** state for the schema contains key `counter`
- **THEN** the expression evaluates to the stored value

#### Scenario RS.ATM.5: Evaluating environment expression
- **WHEN** a message example contains expression `{$env.SERVICE_NAME}`
- **AND** the environment variable `SERVICE_NAME` is set
- **THEN** the expression evaluates to the environment variable value

### Requirement: x-mock-match example selection
The mock server SHALL select AsyncAPI message examples using the `x-mock-match` extension (with legacy `x-mock-params-match` handling) evaluated against the incoming message, mirroring OpenAPI behavior.

#### Scenario RS.ATM.6: Selecting message example by x-mock-match
- **WHEN** multiple message examples exist and exactly one has `x-mock-match` conditions satisfied by the client message
- **THEN** the server selects that example

#### Scenario RS.ATM.7: Selecting first example with no conditions
- **WHEN** an operation has multiple message examples without `x-mock-match`/`x-mock-params-match`
- **THEN** the server selects the first example (by AsyncAPI definition order)

#### Scenario RS.ATM.8: x-mock-match overrides deprecated alias
- **WHEN** a message example has both `x-mock-match` and `x-mock-params-match`
- **THEN** only `x-mock-match` is considered and a deprecation error is written to stderr

### Requirement: x-mock-skip and x-mock-once on message examples
The mock server SHALL honor `x-mock-skip` and `x-mock-once` on AsyncAPI message examples with the same semantics as OpenAPI.

#### Scenario RS.ATM.9: Skipping a message example
- **WHEN** a message example has `x-mock-skip: true`
- **THEN** the server skips that example during selection

#### Scenario RS.ATM.10: One-time message example removal
- **WHEN** a message example with `x-mock-once: true` is selected
- **THEN** the server removes it from future consideration

### Requirement: State mutation via x-mock-set-state
The mock server SHALL apply `x-mock-set-state` from a matched AsyncAPI message example to the schema's state namespace, including increment and delete semantics.

#### Scenario RS.ATM.11: Setting state from message example
- **WHEN** a selected message example has `x-mock-set-state` containing key-value pairs
- **THEN** the server updates the schema state with those pairs

#### Scenario RS.ATM.12: Incrementing state from message example
- **WHEN** `x-mock-set-state` contains `{ counter: { increment: 1 } }`
- **AND** previous `counter` value is a number
- **THEN** the server increments `counter` by 1

#### Scenario RS.ATM.13: Deleting state key from message example
- **WHEN** `x-mock-set-state` contains `key: null`
- **THEN** the server removes `key` from state

### Requirement: x-mock-headers on message responses
The mock server SHALL apply `x-mock-headers` from a matched AsyncAPI message example to the outgoing message/response envelope.

#### Scenario RS.ATM.14: Response headers from message example
- **WHEN** a selected message example has `x-mock-headers`
- **THEN** the server includes those headers in the outgoing message/response

### Requirement: State and history integration for AsyncAPI traffic
Message handling SHALL record request/response history records and use the same state store as OpenAPI traffic, keeping each AsyncAPI schema isolated in its own namespace.

#### Scenario RS.ATM.15: Recording AsyncAPI message exchanges in history
- **WHEN** a ws/http message is processed against an AsyncAPI channel
- **THEN** the exchange is recorded in the request history with channel/address, headers, payload, and timestamp

#### Scenario RS.ATM.16: AsyncAPI state namespace isolation
- **WHEN** multiple AsyncAPI schemas are served
- **THEN** each schema's `x-mock-set-state` writes go to its own isolated state namespace