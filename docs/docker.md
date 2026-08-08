# Docker

OASMock is published as a Docker image on Docker Hub: [`itmamonth/oasmock`](https://hub.docker.com/r/itmamonth/oasmock).

The image is built on a hardened [distroless](https://github.com/GoogleContainerTools/distroless) base (`gcr.io/distroless/static-debian12:nonroot`), runs as a non‑root user, and contains no shell or package manager. It is built and published automatically on version tags via GitHub Actions, so the image content matches the binaries that passed the integration tests.

## Quick Start

```bash
docker pull itmamonth/oasmock:latest
```

Create a `.oasmock.yaml` config file and mount it together with your OpenAPI schemas:

```yaml
# .oasmock.yaml
schemas:
  - src: /schemas/api.yaml
    prefix: /v1
port: 8080
```

```bash
docker run -v $(pwd)/.oasmock.yaml:/app/.oasmock.yaml \
           -v $(pwd)/schemas:/schemas:ro \
           -p 8080:8080 \
           itmamonth/oasmock:latest
```

## Image Tags

| Tag | Description |
|-----|-------------|
| `latest` | Most recent release |
| `vX.Y.Z` | Exact version (e.g. `v0.0.6`) |
| `X.Y` | Minor version tag (e.g. `0.0`) |
| `X` | Major version tag (e.g. `0`) |

Tags are generated from git version tags using the semantic versioning pattern.

## Configuration

The mock server is configured with a YAML file, exactly as documented in the [CLI reference](./cli.md). Inside the container the working directory is `/app`, so mount your config at `/app/.oasmock.yaml`:

```bash
docker run \
  -v $(pwd)/.oasmock.yaml:/app/.oasmock.yaml \
  -v $(pwd)/schemas:/schemas:ro \
  -p 19191:19191 \
  itmamonth/oasmock:latest
```

The config file references schema files by absolute path — mount them at a known location (e.g. `/schemas`) and use that path in `schemas[].src`.

### Environment Variables

Every CLI flag has a matching `OASMOCK_*` environment variable (see [docs/cli.md](./cli.md)), so you can configure the container without a config file:

```bash
docker run -e OASMOCK_PORT=8080 \
           -v $(pwd)/schemas:/schemas:ro \
           itmamonth/oasmock:latest \
           --from /schemas/api.yaml --prefix /v1
```

### Port

The default port is `19191` (matching the CLI default). Publish it to the host with `-p`:

```bash
docker run -p 19191:19191 itmamonth/oasmock:latest --from /schemas/api.yaml
```

To use another port, set it in the YAML config or via `OASMOCK_PORT` and publish it accordingly.

## Docker Compose

```yaml
services:
  oasmock:
    image: itmamonth/oasmock:latest
    ports:
      - "8080:8080"
    volumes:
      - ./oasmock.yaml:/app/.oasmock.yaml:ro
      - ./schemas:/schemas:ro
    restart: unless-stopped
```

## Building Locally

The image packages a pre‑built binary, so build the binary first, then the image:

```bash
make docker-build
```

This compiles a `linux/amd64` binary, builds the image as `oasmock:dev`, and cleans up the intermediate binary.

## Multi-Platform

The published image supports `linux/amd64` and `linux/arm64`. A multi‑architecture manifest is pushed on release, so `docker pull` resolves the correct image for the host automatically.

## Security Notes

- **Hardened base**: The image is based on `gcr.io/distroless/static-debian12:nonroot` — no shell, no package manager, minimal attack surface.
- **Non‑root**: The container runs as an unprivileged user (UID 65532).
- **Read‑only mounts**: Mount schema files and config with `:ro` to prevent modification.
- **No secrets**: Do not bake credentials into the image; use environment variables or mounted secrets.
