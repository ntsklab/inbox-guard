# AGENTS.md

## Project Overview

`inbox-guard` is an ActivityPub inbox spam filter that runs as a reverse proxy in front of `/inbox` endpoints for Mastodon, Misskey, and other ActivityPub-compatible servers. It filters incoming ActivityPub messages before they reach the backend server.

## Development Environment

- **Language:** Go
- **Go version:** Use the latest stable version via the official `golang` Docker image
- **Build & Run:** All development is done inside Docker containers
- **Dependencies:** Zero external dependencies — standard library only

### Build

```bash
docker build -t inbox-guard .
```

### Run tests

```bash
docker run --rm -v "$PWD":/src -w /src golang:latest go test ./...
```

### Build binary (without Docker)

```bash
# For quick local iteration (requires Go installed)
CGO_ENABLED=0 go build -o inbox-guard .
```

## Project Structure

```
inbox-guard/
├── main.go              # Entry point, HTTP server, proxy logic, graceful shutdown
├── config.go            # Environment variable configuration
├── filter.go            # Filter chain builder and filter implementations
├── filter_test.go       # Tests for filters
├── metrics.go           # Request metrics (Prometheus-compatible /metrics endpoint)
├── version.go           # Version variable (injected via -ldflags at build time)
├── filters/
│   └── filter.go        # Filter interface and helper
├── Dockerfile           # Multi-stage build (golang → scratch)
├── go.mod               # Module definition
├── VERSION              # Current version number (source of truth)
├── README.md            # User-facing documentation
└── AGENTS.md            # This file
```

### Key Concepts

- **Filters** implement the `filters.Filter` interface (`Check(content, actor string, r *http.Request) string`). Return `""` to allow, or a reason string to block.
- **Filter chain** is built in `filter.go` → `buildFilterChain()` based on config.
- **Config** is loaded entirely from environment variables (see `config.go`).
- **Proxy** forwards allowed POST requests to the `BACKEND` URL; non-POST requests bypass filters entirely.
- **Metrics** are exposed at `/metrics` in Prometheus text format. Request counts are tracked via atomic counters.
- **Graceful shutdown** handles SIGTERM/SIGINT, draining in-flight requests before stopping.

## Code Conventions

- Standard library only — do not add third-party dependencies.
- New filters go in `filter.go` or new files in `filters/`.
- Tests live alongside the code in `*_test.go` files (package `main` for internal tests, `filters_test` for external).
- Run `gofmt` before committing.
- Always strip debug symbols in production builds: use `-ldflags="-s -w"` with `go build`.
- **Dockerfile** must be kept in sync:
  - If `go.mod` version changes, update `FROM golang:X.XX-alpine` to match.
  - If new source directories or files are added, ensure they are covered by `COPY . .` or add explicit `COPY` steps.
  - If `LISTEN_PORT` default changes, update `EXPOSE` to match.
  - If the entrypoint or build flags change, update `ENTRYPOINT` / `CMD` / `RUN` accordingly.
- **No backward compatibility**: when changing config values, API behavior, or defaults, do not maintain aliases or compatibility shims for old values. Cut over cleanly.

## Git Workflow

- Make focused, atomic commits with clear messages.
- Commit after each logical change (e.g., bug fix, new filter, config change).
- Use English for commit messages.
- Only push when explicitly instructed by the user, as pushing triggers CI workflow actions.
- Message format: `area: concise description`
  - Examples: `config: remove unsafe default backend URL`, `filters: add ratio-based mention detector`
- Version bumping:
  - No breaking changes → patch increment (`0.9.x` → `0.9.x+1`)
  - New feature → minor increment (`0.9.x` → `0.10.0`)
  - When bumping version, update version references in:
    - `VERSION` — version number (source of truth)
    - `README.md` — container image tag
    - `inbox-guard.yaml.sample` — Deployment image tag

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `BACKEND` | **Yes** | — | Backend server URL to proxy to |
| `LISTEN_PORT` | No | `3000` | Port to listen on |
| `ACTION` | No | `403` | HTTP status code on block: `403` (reject), `200` (silent), or any 200–599 |
| `MAX_MENTIONS` | No | `4` | Maximum allowed @mentions |
| `MAX_CONTENT_RATIO` | No | `0.9` | Max mention-to-content ratio |
| `BLOCK_KEYWORDS` | No | — | Comma-separated keywords/URLs to block |
| `BLOCK_DOMAINS` | No | — | Comma-separated domains to block |
| `LOG_LEVEL` | No | `info` | `info` or `debug` |
| `READ_TIMEOUT` | No | `10s` | Max duration for reading the entire request |
| `WRITE_TIMEOUT` | No | `30s` | Max duration before timing out writes |
| `IDLE_TIMEOUT` | No | `60s` | Max time to wait for next request on keep-alive connection |
| `SHUTDOWN_TIMEOUT` | No | `30s` | Max time to wait for in-flight requests during graceful shutdown |

> **Note:** `BACKEND` is mandatory. The previous default `http://localhost:3000` was removed to prevent infinite proxy loops. The program will exit with an error if `BACKEND` is not set.
