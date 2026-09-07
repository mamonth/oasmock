## 1. OpenAPI contract

- [x] 1.1 Remove the `match` property from `NewExampleRequestAsync` in `api/openapi.yaml` and verify `openspec validate` / spectral lint passes on the document
- [x] 1.2 Reword the `conditions` description in `NewExampleRequestBase` to document both reply contexts (`{$request.*}`/`{$message.*}`/`{$channel.*}`) and async event/connection contexts (`{$event.*}`/`{$connection.*}`) and verify the YAML is valid

## 2. Regenerate the runtime schema

- [x] 2.1 Run `make gen` (gen-control-schema) to regenerate `internal/server/add_example_request_schema_gen.go` and verify the generated file no longer contains a `match` property and `go build ./...` succeeds

## 3. Handler refactor (server_management.go)

- [x] 3.1 Remove the `Match map[string]any` field from the `addExampleRequest` struct and update construction sites so only `Conditions` remains; verify compile
- [x] 3.2 In `registerAsyncRuntimeExample`, map `req.Conditions` onto `ext["x-mock-match"]` (instead of `req.Match`) and verify the async registration follows RS.MAPI.24/26/33
- [x] 3.3 Update `needsRuntimeRegistration` to `len(req.Conditions) > 0 || req.Interval > 0`; update `resolveExampleTarget` and the single-trigger guard in `decodeAddExampleRequest` to read `req.Conditions`; verify `go build ./...`

## 4. Tests (TDD order per AGENTS.md)

- [x] 4.1 Update existing async handler tests that send the `match` field to send `conditions` (`internal/server/server_test.go` and async handler tests) and verify they pass
- [x] 4.2 Add a behavioral test that an async target with reply-context `conditions` is honored (RS.MAPI.36); verify it passes
- [x] 4.3 Add a validation test that a request body containing `match` is rejected with HTTP 400 (RS.MAPI.37); verify it passes
- [x] 4.4 Add/adjust unit tests in `internal/server` covering async event- and connection-based `conditions` (RS.MAPI.24, RS.MAPI.26, RS.MAPI.33) and the dual/invalid trigger guard (RS.MAPI.29); verify `go test ./internal/server/... ./internal/extensions/...` passes

## 5. Specs and docs

- [x] 5.1 Update `openspec/specs/management-api/spec.md` async requirements/scenarios from `match` to `conditions` (the change's delta spec already carries the new wording; reconcile the main spec on archive) and verify `openspec validate` passes
- [x] 5.2 Update `docs/extensions.md` "Runtime matches and timing (management API)" wording from `match` to `conditions` and verify the reference to `api/openapi.yaml` is consistent

## 6. Verification

- [x] 6.1 Run `go generate ./internal/server`, `go build ./...`, and `go test ./internal/server/... ./internal/extensions/...`; verify all pass
- [x] 6.2 Run spectral lint on `api/openapi.yaml` and `gitnexus detect-changes --scope all --repo .`; verify a clean result with the documented diff surface (server_management.go, generated schema, openapi.yaml, docs)
