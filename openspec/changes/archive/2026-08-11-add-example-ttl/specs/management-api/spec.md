## MODIFIED Requirements

### Requirement: Add example endpoint
The mock server SHALL provide `POST /_mock/examples` to add a custom mock example.

#### Scenario RS.MAPI.2: Adding a simple example
- **WHEN** a POST request is sent to `/_mock/examples` with a valid `AddExampleRequest` JSON body
- **THEN** the server stores the example and responds with `AddExampleResponse` containing success and an example ID

#### Scenario RS.MAPI.3: Adding a conditional example
- **WHEN** the request includes `conditions` object with runtime expressions
- **THEN** the server stores the example and will match it only when conditions are satisfied

#### Scenario RS.MAPI.4: Adding a one-time example
- **WHEN** the request includes `once: true`
- **THEN** the server stores the example as one-time (disposed after first match)

#### Scenario RS.MAPI.5: Adding an example with validation disabled
- **WHEN** the request includes `validate: false`
- **THEN** the server does not validate the example data against the OpenAPI schema

#### Scenario RS.MAPI.6: Invalid request body
- **WHEN** the request body is missing required fields or malformed
- **THEN** the server responds with HTTP 400

## ADDED Requirements

### Requirement: Add example with TTL
The `AddExampleRequest` SHALL accept an optional `ttl` field (integer seconds).

#### Scenario RS.MAPI.16: Adding an example with TTL
- **WHEN** a POST request is sent to `/_mock/examples` with `ttl: 1`
- **THEN** the server accepts the request and stores the TTL value alongside the example

#### Scenario RS.MAPI.17: TTL field validation — negative value
- **WHEN** a POST request is sent to `/_mock/examples` with `ttl: -1`
- **THEN** the server responds with HTTP 400

#### Scenario RS.MAPI.18: TTL field is optional (omitted)
- **WHEN** a POST request is sent to `/_mock/examples` without a `ttl` field
- **THEN** the server accepts the request and the example has no expiration
