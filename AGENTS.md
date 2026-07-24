# AGENTS.md

## Project Overview

`inbox-guard` is an ActivityPub inbox spam filter that runs as a reverse proxy in front of `/inbox` endpoints for Hollo and Mastodon/Misskey-compatible servers. It filters incoming ActivityPub messages before they reach the backend server.

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
├── main.go              # Entry point, HTTP server, proxy logic
├── config.go            # Environment variable configuration
├── filter.go            # Filter chain builder and filter implementations
├── filter_test.go       # Tests for filters
├── filters/
│   └── filter.go        # Filter interface and helper
├── Dockerfile           # Multi-stage build (golang → scratch)
├── go.mod               # Module definition
├── README.md            # User-facing documentation
└── AGENTS.md            # This file
```

### Key Concepts

- **Filters** implement the `filters.Filter` interface (`Check(content, actor string, r *http.Request) string`). Return `""` to allow, or a reason string to block.
- **Filter chain** is built in `filter.go` → `buildFilterChain()` based on config.
- **Config** is loaded entirely from environment variables (see `config.go`).
- **Proxy** forwards allowed POST requests to the `BACKEND` URL; non-POST requests bypass filters entirely.

## Code Conventions

- Standard library only — do not add third-party dependencies.
- New filters go in `filter.go` or new files in `filters/`.
- Tests live alongside the code in `*_test.go` files (package `main` for internal tests, `filters_test` for external).
- Run `gofmt` before committing.
- Keep the Docker image minimal (target `scratch`, < 10MB).
- **Dockerfile** must be kept in sync:
  - If `go.mod` version changes, update `FROM golang:X.XX-alpine` to match.
  - If new source directories or files are added, ensure they are covered by `COPY . .` or add explicit `COPY` steps.
  - If `LISTEN_PORT` default changes, update `EXPOSE` to match.
  - If the entrypoint or build flags change, update `ENTRYPOINT` / `CMD` / `RUN` accordingly.

## Git Workflow

- Make focused, atomic commits with clear messages.
- Commit after each logical change (e.g., bug fix, new filter, config change).
- Use English for commit messages.
- Message format: `area: concise description`
  - Examples: `config: remove unsafe default backend URL`, `filters: add ratio-based mention detector`

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `BACKEND` | **Yes** | — | Backend server URL to proxy to |
| `LISTEN_PORT` | No | `3000` | Port to listen on |
| `ACTION` | No | `soft` | `soft` (return 200) or `block` (return 403) |
| `MAX_MENTIONS` | No | `4` | Maximum allowed @mentions |
| `MAX_CONTENT_RATIO` | No | `0.9` | Max mention-to-content ratio |
| `BLOCK_KEYWORDS` | No | — | Comma-separated keywords/URLs to block |
| `BLOCK_DOMAINS` | No | — | Comma-separated domains to block |
| `LOG_LEVEL` | No | `info` | `info` or `debug` |

> **Note:** `BACKEND` is mandatory. The previous default `http://localhost:3000` was removed to prevent infinite proxy loops. The program will exit with an error if `BACKEND` is not set.
