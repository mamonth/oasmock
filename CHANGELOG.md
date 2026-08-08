# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed
- Docker image `/app/oasmock` is now marked executable — GitHub artifact downloads strip exec bits, breaking `ENTRYPOINT` in the published image
- CI-built binaries are now statically linked (`CGO_ENABLED=0`) — previously `linux/amd64` was dynamically linked against glibc, causing `exec /app/oasmock: no such file or directory` in the `distroless/static` image
- Release Docker image is now smoke-tested (starts and serves the control API) before it is pushed to Docker Hub, via a shared `smoke-test-image` action also used by the PR `docker-build` check

### Added
- Initial release of OASMock - OpenAPI mock server
- Support for OpenAPI 3.0 schemas with custom extensions
- Runtime expression evaluation with modifiers
- CLI with environment variable support
- Management HTTP API for dynamic examples and request history
- State management and request history recording
- CORS support and configurable request delays