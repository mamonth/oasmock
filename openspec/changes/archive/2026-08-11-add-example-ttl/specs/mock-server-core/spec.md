## ADDED Requirements

### Requirement: Dynamic example TTL expiration
Dynamic examples added with a TTL SHALL be skipped during selection once the TTL has elapsed.

#### Scenario RS.MSC.39: Storing an example with TTL
- **WHEN** a dynamic example is added with a `ttl` in seconds
- **THEN** the server stores the example with its creation timestamp and the specified TTL

#### Scenario RS.MSC.40: Selecting a non-expired TTL example
- **WHEN** a dynamic example has a TTL that has not yet elapsed
- **AND** a request matches that example's route
- **THEN** the example is selected and returned normally

#### Scenario RS.MSC.41: Skipping an expired TTL example
- **WHEN** a dynamic example's TTL has elapsed
- **AND** a request matches that example's route
- **THEN** the expired example is skipped during selection and not returned

#### Scenario RS.MSC.42: TTL=0 means no expiration
- **WHEN** a dynamic example is added with `ttl: 0` or no TTL
- **THEN** the example never expires and behaves as before this change

#### Scenario RS.MSC.43: TTL and once combined
- **WHEN** a dynamic example has both a TTL and `once: true`
- **AND** the example is matched by a request before the TTL elapses
- **THEN** the example is consumed via the once flag and not returned again

### Requirement: Background TTL cleanup
The server SHALL periodically remove expired dynamic examples from memory via a background goroutine.

#### Scenario RS.MSC.44: Sweeping expired examples from storage
- **WHEN** a dynamic example with a TTL has expired
- **AND** the background sweep runs
- **THEN** the expired example is removed from the dynamic examples storage

#### Scenario RS.MSC.45: Cleaning onceExamples on sweep
- **WHEN** an expired TTL example is removed by the background sweep
- **THEN** its corresponding onceExamples entry (if any) is also removed

#### Scenario RS.MSC.46: Preserving non-expired examples
- **WHEN** the background sweep runs
- **AND** examples have not yet expired or have no TTL
- **THEN** those examples remain in storage

#### Scenario RS.MSC.47: Debug logging on removal
- **WHEN** an expired example is removed by the background sweep
- **AND** verbose mode is enabled
- **THEN** a debug-level log entry is emitted containing the route key and example details

#### Scenario RS.MSC.48: Sweep starts on server creation
- **WHEN** a server instance is created
- **THEN** the background goroutine for TTL sweeping is launched

#### Scenario RS.MSC.49: Sweep stops on server shutdown
- **WHEN** the server shuts down
- **THEN** the background goroutine for TTL sweeping is stopped
