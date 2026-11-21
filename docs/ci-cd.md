# CI/CD Pipeline

## Overview

The OASMock CI/CD pipeline is a unified GitHub Actions workflow that ensures code quality, test coverage, and reliable releases. The pipeline follows the principle "test what you ship" by building binaries once and reusing the exact same binaries through integration testing and release.

## Key Design Principles

1. **Single Build Artifact**: Binaries are built once, stored as artifacts, and reused across jobs
2. **Parallel Fast Checks**: Unit tests and spec coverage checks run in parallel for faster feedback
3. **Integration Testing Against Built Binary**: Integration tests run against the actual binary that will be shipped
4. **Same Binary for Release**: The release process uses the exact same binaries that passed integration tests
5. **Cross-Platform Support**: Builds for Linux, macOS, and Windows
6. **Prepare Before Publish**: every release preparation step (npm packaging, Docker metadata, image smoke test) completes before anything is published, and publishing only starts once all preparation jobs are green

## Pipeline Architecture

```mermaid
flowchart TD
    subgraph P1["Parallel Preparation & Fast Checks"]
        direction LR
        A[Unit Tests & Coverage]
        B[Spec Coverage Check]
        B2[Spec Validation<br/>Spectral + codegen freshness]
    end

    subgraph P2["Build & Package"]
        C[Build Binaries<br/>Cross-compile for:<br/>• Linux amd64/arm64<br/>• macOS amd64<br/>• Windows amd64]
        D[Upload Artifacts]
    end

    subgraph P3["Integration Testing"]
        E[Download Linux Binary]
        F[Run Integration Tests]
    end

    subgraph P3b["Docker PR Check"]
        G2[Download Binaries]
        H2[Build & Smoke Test Image]
    end

    subgraph P4["Release Preparation (tags only)"]
        G0[Download Binaries]
        G1[Create npm Package + Generate Docker Metadata]
        G2b[Build & Smoke Test Image]
    end

    subgraph P5["Release Publishing (tags only)"]
        G[Create GitHub Release]
        H[Publish npm Package]
        J[Push Docker Image<br/>linux/amd64 + linux/arm64]
    end

    A --> C
    B --> C
    B2 --> C
    C --> D
    D --> E
    E --> F
    D --> G2
    G2 --> H2
    F --> G0
    G0 --> G1
    G1 --> G2b
    G2b --> G
    G --> H
    H --> J
```

## Trigger Events

| Event | Description | Jobs Executed |
|-------|-------------|---------------|
| `push` to `main` | Code changes to main branch | All jobs except `docker-build` (PR-only), `release-prepare`, and `release` (tag-only) |
| `pull_request` to `main` | Pull request creation/update | All jobs except `release-prepare` and `release` |
| `push` of `v*` tag | Version tag push (e.g., v1.0.0) | All jobs, including `release-prepare` then `release` |

## Job Details

### 1. Unit Tests & Coverage

**Job Name:** `unit-tests`  
**Runs On:** `ubuntu-latest`  
**Purpose:** Validate code quality and test coverage

**Steps:**
1. Checkout code
2. Set up Go 1.23
3. Install dependencies (`go mod tidy`)
4. Run unit tests with coverage check (`make coverage-unit`)
5. Run linter (`golangci-lint`)

**Coverage Requirement:** Minimum 70% code coverage (current baseline)

### 2. Spec Coverage Check

**Job Name:** `spec-coverage`  
**Runs On:** `ubuntu-latest`  
**Purpose:** Ensure all requirement scenarios are covered by tests

**Steps:**
1. Checkout code with full history (for spec analysis)
2. Set up Python
3. Run spec coverage analysis (`scripts/analyze_scenario_coverage.py`)
4. Upload coverage report as artifact

**Coverage Requirement:** 100% scenario coverage (0.999 threshold)

### 3. Spec Validation

**Job Name:** `spec-validation`  
**Runs On:** `ubuntu-latest`  
**Purpose:** Lint the OpenAPI/AsyncAPI specs and verify generated artifacts are current

**Steps:**
1. Checkout code
2. Set up Node.js
3. Lint `api/openapi.yaml` and `api/asyncapi.yaml` with Spectral
4. Set up Go 1.23
5. Regenerate the control schema and fail on drift (`git diff --exit-code`)

### 4. Build Binaries

**Job Name:** `build`  
**Runs On:** `ubuntu-latest`  
**Needs:** `unit-tests`, `spec-coverage`, `spec-validation`  
**Purpose:** Cross-compile binaries for all target platforms

**Steps:**
1. Checkout code
2. Set up Go 1.23
3. Extract version from git tag or describe
4. Cross-compile with version embedded and `CGO_ENABLED=0` (statically linked, required for the `distroless/static` runtime):
   - `oasmock-linux-amd64` (Linux x86_64)
   - `oasmock-linux-arm64` (Linux arm64)
   - `oasmock-darwin-amd64` (macOS x86_64)
   - `oasmock-windows-amd64.exe` (Windows x86_64)
5. Upload binaries as GitHub Actions artifact

**Outputs:** `version` (extracted from git)

### 5. Integration Tests

**Job Name:** `integration-tests`  
**Runs On:** `ubuntu-latest`  
**Needs:** `build`  
**Purpose:** Test the built binary as a complete system

**Environment:** `OASMOCK_TEST_SKIP_BUILD=true` (use pre-built binary)

**Steps:**
1. Checkout code
2. Set up Go 1.23
3. Download binaries artifact to `bin/` directory
4. Make Linux binary executable (`chmod +x`)
5. Create symlink `bin/oasmock` → `bin/oasmock-linux-amd64`
6. Run integration tests (`go test ./test/...`)

**Note:** Tests run against the exact binary that will be released

### 6. Docker Image Build (PR Check)

**Job Name:** `docker-build`  
**Runs On:** `ubuntu-latest`  
**Needs:** `build`  
**When:** Pull requests to `main` only  
**Purpose:** Verify the Dockerfile builds successfully and the packaged binary starts

**Steps:**
1. Checkout code
2. Download binaries artifact into the build context root
3. Make Linux binaries executable (`chmod +x`)
4. Set up Docker Buildx
5. Build and smoke test the image (shared `smoke-test-image` action): builds with the same `build-push-action` config as the release push, loads it locally, starts the container and verifies the control API responds on `:19191`

**Note:** This job is build-only — no image is pushed to any registry.

### 7. Release Preparation

**Job Name:** `release-prepare`  
**Runs On:** `ubuntu-latest`  
**Needs:** `integration-tests` (transitively `build`, `unit-tests`, `spec-coverage`, `spec-validation`)  
**When:** Only on tag pushes (`v*`)  
**Permissions:** `contents: read`  
**Purpose:** Perform every release-side preparation step without publishing anything, so a preparation failure can never leave a partially published release

**Steps:**
1. Checkout code with full history
2. Download binaries artifact
3. Make Linux binaries executable (`chmod +x`)
4. Verify npm authentication
5. Create npm package structure:
   - Copy binaries to `npm-package/bin/`
   - Create `install.js` script for platform detection
   - Generate `package.json` from template with version placeholder
   - Copy documentation files (`README.md`, `LICENSE`, `CHANGELOG.md`, `docs/`, `api/`)
6. Upload the npm package as the `oasmock-npm-package` artifact
7. Generate Docker tags/labels from the git tag (`latest`, `vX.Y.Z`, `X.Y`, `X`)
8. Copy linux binaries into the build context
9. Build and smoke test the image (shared `smoke-test-image` action) — no push

**Outputs:** `version`, `tags`, `labels` (consumed by the publishing job)

### 8. Release Publishing

**Job Name:** `release`  
**Runs On:** `ubuntu-latest`  
**Needs:** `release-prepare`  
**When:** Only on tag pushes (`v*`)  
**Permissions:** `contents: write`, `packages: write`, `id-token: write`  
**Purpose:** Publish only after every preparation job has succeeded; no npm or Docker publication happens while any preparation is failing

**Steps:**
1. Checkout code with full history
2. Download binaries and npm package artifacts
3. Make Linux binaries executable (`chmod +x`)
4. Create or update GitHub release with all four binaries (handles auto-created drafts)
5. Publish to npm registry (skips if the version already exists)
6. Set up Docker Buildx and QEMU (for multi-platform emulation)
7. Log in to Docker Hub
8. Build and push multi-platform image (`linux/amd64`, `linux/arm64`) to `itmamonth/oasmock`, reusing the tags/labels from preparation and the smoke-tested layers via GHA build cache

**Secrets Required:**
- `GITHUB_TOKEN` (auto-provided by GitHub Actions)
- `NPM_TOKEN` (stored in repository secrets)
- `DOCKER_USERNAME` (Docker Hub account name)
- `DOCKER_TOKEN` (Docker Hub access token)

## Artifact Flow

```mermaid
flowchart LR
    subgraph BUILD["Build Job"]
        A[dist/oasmock-linux-amd64]
        B[dist/oasmock-linux-arm64]
        C[dist/oasmock-darwin-amd64]
        D[dist/oasmock-windows-amd64.exe]
    end

    subgraph ARTIFACTS["GitHub Artifacts"]
        E[oasmock-binaries]
        E2[oasmock-npm-package]
    end

    subgraph INTEG["Integration Tests"]
        F[bin/oasmock-linux-amd64]
    end

    subgraph DOCKERPR["Docker PR Check"]
        G[oasmock:pr-test]
    end

    subgraph RELPREP["Release Preparation"]
        H0[dist/* + npm-package/]
        H1[oasmock:smoke-test]
    end

    subgraph RELPUB["Release Publishing"]
        H[dist/* + npm-package/*]
    end

    subgraph GH["GitHub Release"]
        I[Release Assets]
    end

    subgraph NPM["npm Registry"]
        J[npm package]
    end

    subgraph DH["Docker Hub"]
        K[itmamonth/oasmock]
    end

    BUILD -->|Upload| E
    E -->|Download linux only| INTEG
    E -->|Download all| DOCKERPR
    E -->|Download all| RELPREP
    RELPREP -->|Create package| E2
    E -->|Download all| RELPUB
    E2 -->|Download| RELPUB
    RELPUB -->|Create release with binaries| GH
    RELPUB -->|Publish package| NPM
    RELPUB -->|Build & push multi-platform| DH
```

## Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `OASMOCK_TEST_SKIP_BUILD` | Skip binary build in integration tests | `false` |
| `GITHUB_TOKEN` | GitHub API authentication | Auto-provided |
| `NPM_TOKEN` | npm registry authentication | Required for releases |
| `DOCKER_USERNAME` | Docker Hub account name | Required for releases |
| `DOCKER_TOKEN` | Docker Hub access token | Required for releases |

## Makefile Targets

The pipeline uses these Makefile targets:

| Target | Description | Used By |
|--------|-------------|---------|
| `coverage-unit` | Run unit tests with coverage check | `unit-tests` job |
| `spec-coverage` | Check requirement scenario coverage | `spec-coverage` job (via script) |
| `build-cross` | Cross-compile for all platforms | Not used directly (CI does cross-compile) |
| `test-integration` | Run integration tests | `integration-tests` job |
| `docker-build` | Build Docker image from local binary | Local development |

## Quality Gates

1. **Unit Test Coverage**: Must meet or exceed 70% (baseline)
2. **Spec Coverage**: Must be 100% (all requirement scenarios covered)
3. **Spec Validation**: OpenAPI/AsyncAPI specs must lint clean and the generated control schema must be current
4. **Linting**: Must pass `golangci-lint` checks
5. **Integration Tests**: All integration tests must pass
6. **Binary Compatibility**: Binaries must pass integration tests
7. **Docker Image Build**: The Docker image must build and the packaged binary must start (PR check)
8. **Docker Image Smoke Test**: The release image must start and serve the control API before it is pushed to Docker Hub
9. **Prepare Before Publish**: npm and Docker publishing run only after every preparation job has succeeded

## Failure Handling

- If unit tests, spec coverage, or spec validation fail, the build job is skipped
- If build fails, integration tests, docker-build, and release preparation are skipped
- If integration tests fail, release preparation is skipped
- If release preparation fails, the publishing job is skipped — nothing is published to npm or Docker Hub (and no GitHub release is created)
- Release preparation and publishing only run on tag pushes
- If the docker-build PR check fails, the Dockerfile is broken and must be fixed before merging

## Maintenance

### Adding New Platforms
To add support for a new platform (e.g., ARM64):
> **Note:** Linux `arm64` is already supported for binaries and Docker images.

1. Update `build` job in `.github/workflows/ci.yml`:
   ```yaml
   GOOS=linux GOARCH=arm64 go build -o dist/oasmock-linux-arm64 ./cmd/oasmock
   ```
2. Update `install.js` in the release preparation job to handle new platform
3. Update npm package.json template in project root (`os` and `cpu` fields) if needed
4. For Docker images, add the platform to the `platforms` list of the `docker/build-push-action` step in the release publishing job

### Changing Coverage Thresholds
- Unit test coverage: Update `scripts/check-coverage.sh` call in `unit-tests` job
- Spec coverage: Update threshold in `spec-coverage` job (currently 0.999)

### Debugging
- Artifacts are retained for 7 days
- Coverage reports are uploaded as artifacts
- Logs are available in GitHub Actions interface

## Related Files

- `.github/workflows/ci.yml` - Main pipeline definition
- `.github/actions/smoke-test-image/action.yml` - Shared Docker image build + smoke test action
- `Makefile` - Build and test targets
- `Dockerfile` - Container image packaging
- `.dockerignore` - Docker build context exclusions
- `scripts/check-coverage.sh` - Coverage check script
- `scripts/analyze_scenario_coverage.py` - Spec coverage analysis
- `test/_shared/binhelper/` - Binary helper for integration tests
