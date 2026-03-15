## ADDED Requirements

### Requirement: OpenAPI schema loading
The mock server SHALL load one or more OpenAPI 3.0 schemas from files specified via `--from` CLI option.

#### Scenario RS.MSC.1: Loading a single schema
- **WHEN** the server starts with `--from api/openapi.yaml`
- **THEN** the server parses the YAML file and validates it as OpenAPI (3.1 or 3.0)

#### Scenario RS.MSC.2: Loading multiple schemas with prefixes
- **WHEN** the server starts with `--from v1.yaml --prefix /v1 --from v2.yaml --prefix /v2`
- **THEN** the server loads both schemas and routes requests under the respective path prefixes

#### Scenario RS.MSC.3: Schema validation failure
- **WHEN** the specified file is not a valid OpenAPI 3.x schema
- **THEN** the server fails to start and exits with code 3

### Requirement: Request routing
The mock server SHALL route incoming HTTP requests to the matching OpenAPI path based on method and path pattern.

#### Scenario RS.MSC.4: Routing to exact path
- **WHEN** a GET request arrives at `/users` and the schema defines a GET operation at `/users`
- **THEN** the server selects that operation for processing

#### Scenario RS.MSC.5: Routing with path parameters
- **WHEN** a GET request arrives at `/users/123` and the schema defines a GET operation at `/users/{id}`
- **THEN** the server extracts `id=123` and selects the operation

#### Scenario RS.MSC.6: Routing with prefix
- **WHEN** a schema is loaded with prefix `/v1` and a request arrives at `/v1/users`
- **THEN** the server matches the operation at `/users` within that schema

#### Scenario RS.MSC.7: No matching operation
- **WHEN** a request arrives at a path/method not defined in any loaded schema
- **THEN** the server responds with HTTP 404

### Requirement: Example selection
The mock server SHALL select an example from the operation's examples collection based on extension conditions.

#### Scenario RS.MSC.8: Selecting correct x-mock-match param when actual and deprecated are provided
- **WHEN** an example has `x-mock-match` parameter
- **AND** an example has `x-mock-params-match` parameter
- **THEN** consider only `x-mock-match` and ignore `x-mock-params-match`, write error message in stderr

#### Scenario RS.MSC.9: Selecting first example when no conditions
- **WHEN** an operation has multiple examples without `x-mock-match` or `x-mock-params-match`
- **THEN** the server selects the first example (by OpenAPI definition order)

#### Scenario RS.MSC.10: Selecting example matching conditions
- **WHEN** an example has `x-mock-match` or `x-mock-params-match` conditions that match the request
- **THEN** the server selects that example

#### Scenario RS.MSC.11: Skipping example with x-mock-skip
- **WHEN** an example has `x-mock-skip: true`
- **THEN** the server skips that example during selection

#### Scenario RS.MSC.12: One-time example removal
- **WHEN** an example with `x-mock-once: true` is selected
- **THEN** the server removes it from future consideration

### Requirement: Runtime expression evaluation
The mock server SHALL evaluate runtime expressions in extension keys and values using request data, state, and environment.

#### Scenario RS.MSC.13: Evaluating path parameter expression
- **WHEN** expression `{$request.path.userId}` appears
- **AND** the request path contains parameter `userId`
- **THEN** the expression evaluates to the parameter value

#### Scenario RS.MSC.14: Evaluating query parameter expression
- **WHEN** expression `{$request.query.page}` appears
- **THEN** the expression evaluates to the query parameter `page`

#### Scenario RS.MSC.15: Evaluating header expression
- **WHEN** expression `{$request.header.accept}` appears
- **THEN** the expression evaluates to the Accept header value

#### Scenario RS.MSC.16: Evaluating body expression
- **WHEN** expression `{$request.body.email}` appears
- **AND** the request body is JSON with property `email`
- **THEN** the expression evaluates to that property

#### Scenario RS.MSC.17: Evaluating cookie expression
- **WHEN** expression `{$request.cookie.session}` appears
- **THEN** the expression evaluates to the session cookie value

#### Scenario RS.MSC.18: Evaluating state expression
- **WHEN** expression `{$state.counter}` appears
- **AND** state contains key `counter`
- **THEN** the expression evaluates to the stored value

#### Scenario RS.MSC.19: Evaluating environment expression
- **WHEN** expression `{$env.NODE_ENV}` appears
- **THEN** the expression evaluates to the environment variable value

### Requirement: State management
The mock server SHALL maintain a mutable state that can be read and written via extensions.

#### Scenario RS.MSC.20: Setting state via x-mock-set-state
- **WHEN** an example with `x-mock-set-state` is matched
- **THEN** the server updates state according to the extension's key-value pairs

#### Scenario RS.MSC.21: Incrementing empty state value with number value
- **WHEN** one of `x-mock-set-state` properties value contains `{ increment: 1 }`
- **AND** previously saved state value for same key is empty (null)
- **THEN** the server sets the corresponding state value to 1 (as number)

#### Scenario RS.MSC.22: Incrementing number state value with positive number value 
- **WHEN** one of `x-mock-set-state` properties value contains `{ increment: 1 }`
- **AND** previously saved state value for same key is a number (in string or number form)
- **THEN** the server increments the corresponding state value by 1

#### Scenario RS.MSC.23: Incrementing number state with negative number value
- **WHEN** one of `x-mock-set-state` properties value contains `{ increment: -1 }`
- **AND** previously saved state value for same key is a number (in string or number form)
- **THEN** the server decrements the corresponding state value by 1

#### Scenario RS.MSC.24: Incrementing string state value with positive number value
- **WHEN** one of `x-mock-set-state` properties value contains `{ increment: 1 }`
- **AND** previously saved state value for same key is a string (non convertable to number)
- **THEN** the server prints error message to stdout

#### Scenario RS.MSC.25: Deleting state key
- **WHEN** one of `x-mock-set-state` properties value contains `key: null`
- **THEN** the server removes `key` from state

#### Scenario RS.MSC.26: State isolation
- **WHEN** multiple schemas are loaded
- **THEN** each schema has its own isolated state namespace

### Requirement: Response generation
The mock server SHALL generate HTTP responses based on the selected example.

#### Scenario RS.MSC.27: Response status code
- **WHEN** the selected example includes a `code` field
- **THEN** the server responds with that HTTP status code

#### Scenario RS.MSC.28: Response headers
- **WHEN** the selected example includes `x-mock-headers`
- **THEN** the server includes those headers in the response

#### Scenario RS.MSC.29: Response body
- **WHEN** the selected example includes a `body` field
- **THEN** the server sends that body as the response content respecting mime type

#### Scenario RS.MSC.30: Content-Type header
- **WHEN** the OpenAPI operation defines a response content type
- **THEN** the server adds a Content-Type header matching that type

### Requirement: Request history recording
The mock server SHALL record all processed requests for later retrieval via the management API.

#### Scenario RS.MSC.31: Recording request details
- **WHEN** a request is processed (regardless of match)
- **THEN** the server stores timestamp, URL, method, headers, and parsed body in history

#### Scenario RS.MSC.32: History size limit
- **WHEN** the number of recorded requests exceeds a configurable limit (default 1000)
- **THEN** the server discards oldest entries to stay within the limit

### Requirement: CORS support
The mock server SHALL include CORS headers in responses unless disabled via `--nocors` or environment.

#### Scenario RS.MSC.33: CORS headers present by default
- **WHEN** a request is processed and CORS is not disabled
- **THEN** the response includes appropriate CORS headers (Access-Control-Allow-Origin: * etc.)

#### Scenario RS.MSC.34: CORS disabled
- **WHEN** the server is started with `--nocors` or OASMOCK_NO_CORS=true
- **THEN** responses do not include CORS headers

### Requirement: Request-response delay
The mock server SHALL introduce a configurable delay between receiving a request and sending a response.

#### Scenario RS.MSC.35: Default delay
- **WHEN** no `--delay` option is provided
- **THEN** the server respond immediately

#### Scenario RS.MSC.36: Custom delay
- **WHEN** the server is started with `--delay 500`
- **THEN** the server waits 500ms before responding

### Requirement: Verbose logging
The mock server SHALL log detailed request/response information when verbose mode is enabled.

#### Scenario RS.MSC.37: Verbose mode enabled via CLI
- **WHEN** the server is started with `--verbose`
- **THEN** the server logs each request's method, path, headers, body, and response details

#### Scenario RS.MSC.38: Verbose mode enabled via environment
- **WHEN** OASMOCK_VERBOSE=true
- **THEN** the server enables verbose logging
