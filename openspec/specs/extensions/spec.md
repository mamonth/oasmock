## Purpose

OpenAPI extensions that enable conditional example selection, state management, dynamic headers, and runtime expression evaluation for the OASMock server.

## Requirements

### Requirement: Mock extension x-mock-match
The mock server SHALL support `x-mock-match` (and `x-mock-params-match` as alias) on OAS example objects to select examples based on request conditions.

#### Scenario RS.EXT.1: Matching example by query parameter value
- **WHEN** an example has `x-mock-match` with `'{$request.query.id}': 12`
- **AND** a request arrives with query parameter `id=12`
- **THEN** the server selects that example for the response

#### Scenario RS.EXT.2: Matching example by JSON schema validation
- **WHEN** an example has `x-mock-match` with a JSON schema for a query parameter
- **AND** a request arrives with a parameter value that validates against the schema
- **THEN** the server selects that example

### Requirement: Mock extension x-mock-skip
The mock server SHALL support `x-mock-skip` to exclude an example from being used for mocking.

#### Scenario RS.EXT.3: Skipping a marked example
- **WHEN** an example has `x-mock-skip: true`
- **THEN** the server never uses that example for responses

### Requirement: Mock extension x-mock-set-state
The mock server SHALL support `x-mock-set-state` to set server state that can be used later in conditions.

#### Scenario RS.EXT.4: Setting state from request body
- **WHEN** an example has `x-mock-set-state` with `state-key: '{$request.body.param}'`
- **AND** a matching request arrives with a body containing `param`
- **THEN** the server stores the value in state under `state-key`

#### Scenario RS.EXT.5: Incrementing a counter state
- **WHEN** an example has `x-mock-set-state` with `counter: { increment: 1 }`
- **AND** the example is matched
- **THEN** the server increments the `counter` state value by 1

#### Scenario RS.EXT.6: Deleting state
- **WHEN** an example has `x-mock-set-state` with `key: null`
- **AND** the example is matched
- **THEN** the server removes `key` from state

### Requirement: Mock extension x-mock-headers
The mock server SHALL support `x-mock-headers` to set response headers for a specific example.

#### Scenario RS.EXT.7: Setting response header from request query
- **WHEN** an example has `x-mock-headers` with `Location: '{$request.query.backUrl}'`
- **AND** a matching request arrives with `backUrl` query parameter
- **THEN** the server includes the evaluated Location header in the response

### Requirement: Mock extension x-mock-once
The mock server SHALL support `x-mock-once` to make an example one-time (disposed after first match).

#### Scenario RS.EXT.8: One-time example
- **WHEN** an example has `x-mock-once: true`
- **AND** a request matches that example
- **THEN** the server uses the example for that request and removes it from further consideration

### Requirement: Runtime expression evaluation
The mock server SHALL evaluate runtime expressions in keys and values of mock extensions.

#### Scenario RS.EXT.9: Accessing path parameter
- **WHEN** an expression `{$request.path.param}` appears in an extension
- **AND** a request arrives with path parameter `param`
- **THEN** the expression evaluates to the parameter value

#### Scenario RS.EXT.10: Accessing request header
- **WHEN** an expression `{$request.header.content-type}` appears
- **THEN** the expression evaluates to the Content-Type header value

#### Scenario RS.EXT.11: Accessing request body property
- **WHEN** an expression `{$request.body.field}` appears
- **AND** the request body is JSON with property `field`
- **THEN** the expression evaluates to the property value

#### Scenario RS.EXT.12: Accessing saved state
- **WHEN** an expression `{$state.savedParam}` appears
- **AND** state contains `savedParam`
- **THEN** the expression evaluates to the stored state value

#### Scenario RS.EXT.13: Accessing environment variable
- **WHEN** an expression `{$env.PORT}` appears
- **THEN** the expression evaluates to the value of environment variable PORT

### Requirement: Runtime expression modifiers
The mock server SHALL support value modifiers after a `|` sign in runtime expressions.

#### Scenario RS.EXT.14: Using default modifier
- **WHEN** expression `{$request.path.param|default:some default value}` evaluates to empty
- **THEN** the result is `some default value`

#### Scenario RS.EXT.15: Using getByPath modifier
- **WHEN** expression `{$state.someObject|getByPath:some.example.array.last}` evaluates
- **AND** `someObject` is a nested object
- **THEN** the result is the value at path `some.example.array.last`

#### Scenario RS.EXT.16: Using toJWT modifier
- **WHEN** expression `{$state.someObject|toJWT}` evaluates
- **THEN** the result is a JWT token containing the object as payload with exp 1h and aud="mock-client"

#### Scenario RS.EXT.17: Escaping dots in property names
- **WHEN** expression `{$request.cookie.dot\.dot}` appears
- **THEN** the dot is treated as part of the property name, not a path separator
