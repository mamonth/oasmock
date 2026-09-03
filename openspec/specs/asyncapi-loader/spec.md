# asyncapi-loader Specification

## Purpose
Auto-detection and loading of AsyncAPI 3.x specifications (3.0.0/3.1.0) with structural validation and multi-schema prefixing.
## Requirements
### Requirement: AsyncAPI specification detection
The loader SHALL detect, for each loaded file, whether it is an OpenAPI or an AsyncAPI specification by inspecting the root-level version key (`openapi` vs `asyncapi`), choosing the corresponding loader automatically without explicit user configuration.

#### Scenario RS.AAL.1: Detecting an OpenAPI specification
- **WHEN** a file contains root key `openapi: 3.1.0`
- **THEN** the loader treats the file as an OpenAPI specification and loads it with the OpenAPI loader

#### Scenario RS.AAL.2: Detecting an AsyncAPI specification
- **WHEN** a file contains root key `asyncapi: 3.0.0`
- **THEN** the loader treats the file as an AsyncAPI specification and loads it with the AsyncAPI loader

#### Scenario RS.AAL.3: Detecting AsyncAPI 3.1.0
- **WHEN** a file contains root key `asyncapi: 3.1.0`
- **THEN** the loader loads the file as an AsyncAPI 3.1.0 specification

#### Scenario RS.AAL.4: File with neither version key
- **WHEN** a file contains neither an `openapi` nor an `asyncapi` root key
- **THEN** the loader reports a schema loading error for that file

### Requirement: AsyncAPI 3.0.0 loading
The loader SHALL load and structurally validate AsyncAPI 3.0.0 specification files (YAML or JSON), resolving inline references within the document.

#### Scenario RS.AAL.5: Loading a valid AsyncAPI 3.0.0 spec
- **WHEN** the server starts with `--from asyncapi30.yaml`
- **AND** the file is a valid AsyncAPI 3.0.0 specification
- **THEN** the server parses and validates the file without error

#### Scenario RS.AAL.6: Invalid AsyncAPI 3.0.0 spec
- **WHEN** the file is missing mandatory AsyncAPI fields (e.g., no `channels`, no `operations`)
- **THEN** the loader reports a schema validation error

### Requirement: AsyncAPI 3.1.0 loading
The loader SHALL load and structurally validate AsyncAPI 3.1.0 specification files (YAML or JSON), which introduce changes such as the `webhooks` component and adjusted `components` requirements.

#### Scenario RS.AAL.7: Loading a valid AsyncAPI 3.1.0 spec
- **WHEN** the server starts with `--from asyncapi31.yaml`
- **AND** the file is a valid AsyncAPI 3.1.0 specification
- **THEN** the server parses and validates the file without error

#### Scenario RS.AAL.8: 3.x version without supported protocol
- **WHEN** an AsyncAPI 3.x file is otherwise valid but contains only unknown protocol bindings (including `amqp`)
- **THEN** the loader reports a validation error naming the unsupported protocol

### Requirement: Multiple AsyncAPI schemas with prefixes
The loader SHALL load multiple AsyncAPI schemas alongside OpenAPI schemas, pairing each source with its prefix in the same way OpenAPI schemas are handled today.

#### Scenario RS.AAL.9: Multiple AsyncAPI schemas with prefixes
- **WHEN** the server starts with `--from async1.yaml --prefix /a1 --from async2.yaml --prefix /a2`
- **THEN** the server loads both AsyncAPI schemas and attaches the respective prefixes to their channels

#### Scenario RS.AAL.10: Mixing OpenAPI and AsyncAPI sources
- **WHEN** the server starts with `--from openapi.yaml --from asyncapi.yaml`
- **THEN** both specifications are loaded through their respective loaders and served together

### Requirement: AsyncAPI model exposure
The loader SHALL expose the loaded AsyncAPI model (channels, operations, messages, components, bindings) to the routing layer in a form that preserves the original structure for further mapping.

#### Scenario RS.AAL.11: Exposing channels and operations
- **WHEN** an AsyncAPI 3.x spec is loaded
- **THEN** the router receives channel, operation, message, and binding definitions derived from the spec

#### Scenario RS.AAL.12: Unsupported AsyncAPI major version
- **WHEN** a file has root key `asyncapi` with a version major other than 3 (e.g., `2.6.0`)
- **THEN** the loader reports a schema validation error stating the version is unsupported

