## Purpose

HTTP management API for runtime control of the OASMock server, allowing dynamic addition of mock examples and retrieval of request history.

## Requirements

### Requirement: Management API availability
The mock server SHALL provide an HTTP API for runtime management under the `/_mock` path prefix according to [openapi](../../../../openapi.yaml) spec

#### Scenario RS.MAPI.1: Accessing management endpoints
- **WHEN** the mock server is running
- **THEN** endpoints `/_mock/examples` and `/_mock/requests` are accessible

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

### Requirement: Request history endpoint
The mock server SHALL provide `GET /_mock/requests` to retrieve request history.

#### Scenario RS.MAPI.7: Retrieving all requests
- **WHEN** a GET request is sent to `/_mock/requests` without query parameters
- **THEN** the server responds with up to 1000 most recent requests (default limit)

#### Scenario RS.MAPI.8: Filtering by path
- **WHEN** a `path` query parameter is provided
- **THEN** the server returns only requests matching that path

#### Scenario RS.MAPI.9: Filtering by method
- **WHEN** a `method` query parameter is provided (GET, POST, etc.)
- **THEN** the server returns only requests with that HTTP method

#### Scenario RS.MAPI.10: Pagination with limit and offset
- **WHEN** `limit` and `offset` query parameters are provided
- **THEN** the server returns at most `limit` requests starting from `offset`

#### Scenario RS.MAPI.11: Filtering by time range
- **WHEN** `time_from` and/or `time_till` query parameters are provided (milliseconds since epoch)
- **THEN** the server returns only requests within the specified time range

#### Scenario RS.MAPI.12: Limit exceeding maximum
- **WHEN** `limit` is greater than 1000
- **THEN** the server caps the limit to 1000

### Requirement: Request history data format
The request history SHALL include timestamp, URL, method, headers, and parsed body.

#### Scenario RS.MAPI.13: Request history item structure
- **WHEN** a request is processed by the mock server
- **THEN** an entry is added to history containing `ts`, `url`, `method`, `headers`, and `body` fields

### Requirement: Example response format
The `AddExampleRequest` and `AddExampleResponse` SHALL follow the schemas defined in openapi.yaml.

#### Scenario RS.MAPI.14: AddExampleRequest validation
- **WHEN** a request includes `path` (string) and `response` (ExampleResponse) fields
- **AND** optional fields `method`, `once`, `validate`, `conditions`
- **THEN** the request is considered valid

#### Scenario RS.MAPI.15: ExampleResponse structure
- **WHEN** an example response is provided
- **THEN** it includes `code` (integer), `headers` (object), and `body` (any JSON value)
