## MODIFIED Requirements

### Requirement: Add example targeting an AsyncAPI route
The mock server SHALL accept a route identifier that resolves to an AsyncAPI channel/operation (protocol + address, and HTTP method/action when applicable) in `POST /_mock/examples`, with the same storage, matching, once/TTL, conditions, and validation semantics as OpenAPI routes. The AsyncAPI target vocabulary SHALL also cover channels served by a SignalR hub: an example may target a hub channel address (e.g. `/qoden/OpenOrders`) with an event/interval/condition trigger, and the server SHALL deliver into that hub's open streams when the trigger fires.

#### Scenario RS.MAPI.19: Adding a dynamic example for an AsyncAPI channel
- **WHEN** a POST request is sent to `/_mock/examples` with an AsyncAPI route identifier (protocol `ws`/`http` and a channel address)
- **THEN** the server stores the example for that channel and responds with `AddExampleResponse` containing success and an example ID

#### Scenario RS.MAPI.38: Adding a dynamic example for a SignalR hub channel
- **WHEN** a POST request is sent to `/_mock/examples` targeting a channel served by a SignalR hub (e.g. `/qoden/OpenOrders`) with an event/interval/condition trigger
- **THEN** the server resolves the target to the hub channel, registers the example, responds with success and an example ID, and delivers into the hub channel's open streams when the trigger fires (matching raw-ws delivery semantics)

#### Scenario RS.MAPI.20: Dynamic example used by AsyncAPI traffic
- **WHEN** a ws/http message arrives for an AsyncAPI channel that has a dynamic example with matching conditions
- **THEN** the server selects the dynamic example using the same selection pipeline as spec examples

#### Scenario RS.MAPI.21: No matching AsyncAPI route
- **WHEN** a POST request is sent to `/_mock/examples` with an AsyncAPI route identifier that does not match any loaded channel (raw ws/http or SignalR hub)
- **THEN** the server responds with HTTP 400 (no matching route)

## ADDED Requirements

### Requirement: SignalR hub consumers are addressable by path
For a hub served at a parameterized path (e.g. `x-signalr.path: /frontoffice/ws/account/{accountId}`), the management API consumer surface SHALL let a caller discover the concrete upgrade path (including path-parameter values such as a per-account `accountId`) of each SignalR connection, so a targeted async push can select the connections of one logical account on a shared hub channel.

#### Scenario RS.MAPI.39: Discovery exposes the hub path value
- **WHEN** a management request lists consumers of a hub channel and multiple SignalR consumers are connected over distinct per-account paths
- **THEN** each consumer record includes the path-parameter value identifying its account, enabling a targeted push to exactly one account's stream
