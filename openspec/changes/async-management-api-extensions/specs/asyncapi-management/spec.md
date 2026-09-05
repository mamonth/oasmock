# asyncapi-management Delta

## Purpose

Runtime driving of async mocking is made protocol-neutral and reactive: consumers can be listed globally, recurring delivery is expressed through the runtime `interval` field (`x-mock-interval` extension) instead of a dedicated schedule endpoint, and a general management WebSocket stream exposes runtime events.

## ADDED Requirements

### Requirement: Management WebSocket event stream
The mock server SHALL expose a general management WebSocket stream at `GET /_mock/stream` (upgrade) through which a client subscribes to runtime notifications. V1 SHALL be notifications-only: the client sets filters at connection time via `events` and `channels` query parameters (comma-separated, `*` wildcard supported), and the server pushes JSON envelopes. A non-upgrade request to `/_mock/stream` SHALL be rejected.

#### Scenario RS.AMG.23: Subscribing with event and channel filters
- **WHEN** a client connects to `/_mock/stream` with `?events=orderCreated&channels=/alerts`
- **THEN** the client receives envelopes only for matching events and channels; an omitted filter matches everything

#### Scenario RS.AMG.24: Receiving an event-fired envelope
- **WHEN** a named event fires (spec-triggered or via the management API) and a subscribed client is connected
- **THEN** the client receives an envelope of type `event` with the event name, payload, schema scope, and global flag

#### Scenario RS.AMG.25: Receiving a push envelope
- **WHEN** a management push delivers a message to a channel
- **THEN** a subscribed client receives an envelope of type `push` with the channel, target connection (when targeted), and payload

#### Scenario RS.AMG.26: Receiving consumer lifecycle envelopes
- **WHEN** a consumer connects to or disconnects from a channel (raw ws or SignalR)
- **THEN** a subscribed client receives an envelope of type `consumer` with a `connected`/`disconnected` action, connection ID, and channel

#### Scenario RS.AMG.27: Receiving schedule start/stop envelopes
- **WHEN** a periodic message example is registered with `interval` via `POST /_mock/examples` (or spec `x-mock-interval`) or removed via `DELETE /_mock/examples/{exampleId}`
- **THEN** a subscribed client receives an envelope of type `schedule` with a `started`/`stopped` action, example ID, channel, and interval

#### Scenario RS.AMG.28: Non-upgrade request to the stream endpoint
- **WHEN** a plain HTTP (non-WebSocket) request is sent to `/_mock/stream`
- **THEN** the server responds with HTTP 405

## MODIFIED Requirements

### Requirement: Connected consumer discovery
The mock server SHALL expose the currently connected consumers per AsyncAPI channel, including open SignalR streams. The `channel` query parameter SHALL be optional: when omitted, the server SHALL return consumers across all channels; when present, it SHALL return only consumers of that channel.

#### Scenario RS.AMG.8: Listing connected consumers
- **WHEN** a management request queries consumers for an AsyncAPI channel with active connections
- **THEN** the server returns the consumer list with connection IDs, channel/address details, and open streams (for SignalR hubs)

#### Scenario RS.AMG.9: Listing consumers for a channel with no connections
- **WHEN** a management request queries consumers for an AsyncAPI channel with no active connections
- **THEN** the server returns an empty list

#### Scenario RS.AMG.22: Listing all consumers without a channel filter
- **WHEN** a management request queries consumers without a `channel` parameter and consumers are connected on multiple channels
- **THEN** the server returns a single flat list of consumers across all channels (raw ws and SignalR), and an empty list when none are connected

## REMOVED Requirements

### Requirement: Recurring scheduled push
The mock server SHALL support scheduling repeated pushes of a message example to a channel at a fixed interval.

#### Scenario RS.AMG.12: Scheduling a recurring push
- **WHEN** a management request schedules a push with an `interval` in milliseconds
- **THEN** the message is delivered repeatedly at that interval until stopped (or the server shuts down)

#### Scenario RS.AMG.13: Stopping a recurring push
- **WHEN** a management request stops a previously scheduled recurring push (by its push ID)
- **THEN** no further deliveries occur for that schedule

**Reason**: The dedicated schedule surface was a single-variant duplicate of the `cron` built-in. Recurring delivery is now expressed uniformly through the runtime `interval` field / `x-mock-interval` extension on message examples, removing the parallel endpoint and its push-ID lifecycle.

**Migration**: Use `POST /_mock/examples` with an AsyncAPI target, `response.body`, and `interval: <ms>` to start recurrence, and `DELETE /_mock/examples/{exampleId}` to stop it. The legacy `/_mock/ws/schedule*` paths respond with HTTP 410 Gone pointing to `POST /_mock/examples`.