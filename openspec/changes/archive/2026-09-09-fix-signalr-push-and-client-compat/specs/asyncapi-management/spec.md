## MODIFIED Requirements

### Requirement: Targeted and broadcast push
The pushed message SHALL be deliverable to a single consumer connection or broadcast to all consumers of the channel. The payload SHALL be any JSON value — object, array, or scalar — delivered verbatim to the target consumer(s) as the channel message / SignalR stream `item`; the server SHALL NOT reject non-object payloads.

#### Scenario RS.AMG.5: Pushing to a specific consumer
- **WHEN** a management request includes a `connectionId` for an active consumer
- **THEN** only that consumer receives the message

#### Scenario RS.AMG.6: Broadcasting to all consumers
- **WHEN** a management request omits `connectionId`
- **THEN** all consumers currently connected to the channel receive the message

#### Scenario RS.AMG.31: Pushing an array payload
- **WHEN** a management request posts an array payload (e.g. `[{"orderId": "grid-1"}]`) to `/_mock/async/messages`
- **THEN** the array is delivered verbatim to the channel's consumers (as the message body / SignalR stream `item`) instead of being rejected

#### Scenario RS.AMG.7: Unknown consumer reference
- **WHEN** a management request includes a `connectionId` that has no active connection
- **THEN** the server responds with HTTP 404

### Requirement: Connected consumer discovery
The mock server SHALL expose the currently connected consumers per AsyncAPI channel, including open SignalR streams. The `channel` query parameter SHALL be optional: when omitted, the server SHALL return consumers across all channels; when present, it SHALL return only consumers of that channel. Each consumer record SHALL carry enough identity to target a push — for SignalR connections this includes the upgrade path (with any captured path parameters such as a per-account `accountId` segment) in addition to the connection id and channel address.

#### Scenario RS.AMG.8: Listing connected consumers
- **WHEN** a management request queries consumers for an AsyncAPI channel with active connections
- **THEN** the server returns the consumer list with connection IDs, channel/address details, the consumer's `protocol` (`ws` for raw WebSocket or `signalr` for SignalR), open streams (for SignalR hubs), and — for a SignalR connection on a parameterized hub path — the concrete upgrade path / path-parameter values

#### Scenario RS.AMG.32: Consumer path metadata for per-account targeting
- **WHEN** two SignalR consumers on the same hub channel connect over different per-account paths and a management request queries the channel's consumers
- **THEN** each consumer record exposes the path value that distinguishes them (e.g. `accountId`), allowing a targeted push to select one account's connection

#### Scenario RS.AMG.9: Listing consumers for a channel with no connections
- **WHEN** a management request queries consumers for an AsyncAPI channel with no active connections
- **THEN** the server returns an empty list

#### Scenario RS.AMG.22: Listing all consumers without a channel filter
- **WHEN** a management request queries consumers without a `channel` parameter and consumers are connected on multiple channels
- **THEN** the server returns a single flat list of consumers across all channels (raw ws and SignalR, each tagged with its `protocol`), and an empty list when none are connected
