## Why

`POST /examples` in the management API exposes two parallel example-selection
fields: the original sync `conditions` (on the shared request base) and the
async-only `match` (added by the async-management-extensions change). The two
are architecturally redundant — `match` merely mirrors the `x-mock-match`
spec-extension vocabulary against async event/connection contexts, while
`conditions` is ignored on async targets and `match` is ignored on sync targets
— so a client can send both (or a nonsense `conditions` on an async body) with
no effect and no documented rationale. This is a schema smell, not a deliberate
design, and it must be collapsed into a single selection field.

## What Changes

- Make `conditions` the single example-selection field for **all** target kinds
  in `POST /examples` (sync `{$request.*}`/`{$message.*}`/`{$channel.*}` reply
  contexts and async `{$event.*}`/`{$connection.*}` event/connection contexts).
- **BREAKING**: Remove the async-only `match` property from the request schema
  (clean drop — no deprecation alias). A request body carrying `match` is now
  rejected by schema validation.
- Async targets now honor `conditions` (previously silent no-op): an event- or
  interval-triggered registration maps `conditions` onto the internal
  `x-mock-match` extension.
- `interval` / `delay` remain async-only timing fields (selector stays pure of
  timing, mirroring the `x-mock-match`/`x-mock-interval`/`x-mock-delay` split).
- Regenerate the runtime validation schema
  (`internal/server/add_example_request_schema_gen.go`) from the OpenAPI
  contract, keeping the document single source of truth.
- Update specs/docs that referenced the `match` wire field.

## Capabilities

### New Capabilities

None. No new behavior domain is introduced.

### Modified Capabilities

- `management-api`: The `POST /examples` contract changes — examples of every
  target kind select through a single `conditions` field; the `match` field is
  removed and async targets accept (and honor) event/connection `conditions`
  instead.

## Impact

- **API**: `api/openapi.yaml` — remove `match` from `NewExampleRequestAsync`;
  re-scope the `conditions` description to cover both context families.
- **Handlers**: `internal/server/server_management.go` — drop `Match` from the
  request struct; route `conditions` into the async runtime registration
  (`registerAsyncRuntimeExample`), and update `resolveExampleTarget`,
  `needsRuntimeRegistration`, and `decodeAddExampleRequest` to read
  `conditions` instead of `match`. Internal selection still uses
  `x-mock-match` (`internal/extensions/match.go`, `classify.go`) unchanged.
- **Generated schema**: `internal/server/add_example_request_schema_gen.go`
  regenerated via `gen-control-schema` (Makefile `gen` target).
- **Specs/docs**: `openspec/specs/management-api/spec.md` (wire field rename of
  async conditions) and `docs/extensions.md` (runtime `conditions` wording).
- **Tests**: update async handler tests that send `match`; add coverage that
  async `conditions` is now honored and that a `match` field is rejected.
