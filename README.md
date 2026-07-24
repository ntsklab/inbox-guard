# inbox-guard

ActivityPub inbox spam filter for Hollo (and any Mastodon/Misskey-compatible server).

Runs as a reverse proxy in front of `/inbox`. Filters messages before they reach your ActivityPub server.

## Features

- **Mention spam detection** — blocks posts with excessive `@mentions` (configurable threshold)
- **Content ratio check** — blocks posts where >90% of content is mentions (no real content)
- **Keyword/URL block** — blocks posts containing specified keywords or URLs
- **Domain block** — blocks posts from specified domains
- **Silent drop (soft)** or **explicit reject (block)** modes
- **Zero dependencies** — standard library only, ~5MB Docker image (from scratch)
- **Health check** endpoint at `/health`
- **Prometheus-compatible metrics** at `/metrics`
- **Horizontal scaling** — stateless, graceful shutdown, configurable timeouts

## Quick start

```yaml
# docker-compose.yml
services:
  inbox-guard:
    build: ./inbox-guard
    ports:
      - "7000:3000"
    environment:
      - BACKEND=http://hollo:3000
      - MAX_MENTIONS=50
      - MAX_CONTENT_RATIO=0.9
      - BLOCK_KEYWORDS=ctkpaarr.org,discord.gg/spam
      - BLOCK_DOMAINS=spam-server.example.com
      - ACTION=soft
      - LOG_LEVEL=info
```

Then route `/inbox` traffic through port 7000.

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `LISTEN_PORT` | `3000` | Port to listen on |
| `BACKEND` | `http://localhost:3000` | Backend server URL (Hollo) |
| `ACTION` | `soft` | `soft` = return 200 (silent drop), `block` = return 403 |
| `MAX_MENTIONS` | `50` | Maximum allowed @mentions in a post |
| `MAX_CONTENT_RATIO` | `0.9` | Max ratio of mention content to total content (0.0–1.0) |
| `BLOCK_KEYWORDS` | (empty) | Comma-separated keywords/URLs |
| `BLOCK_DOMAINS` | (empty) | Comma-separated domains to block |
| `LOG_LEVEL` | `info` | `info` or `debug` |
| `READ_TIMEOUT` | `10s` | Max duration for reading the entire request |
| `WRITE_TIMEOUT` | `30s` | Max duration before timing out writes |
| `IDLE_TIMEOUT` | `60s` | Max time to wait for next request on a keep-alive connection |
| `SHUTDOWN_TIMEOUT` | `30s` | Max time to wait for in-flight requests during graceful shutdown |

## Adding custom filters

1. Create a new file in `filters/` implementing the `Filter` interface:

```go
package filters

type MyFilter struct { /* config */ }

func (f *MyFilter) Check(content, actor string, r *http.Request) string {
    // Return "" if allowed, "reason string" if blocked
    return ""
}
```

2. Register it in `filter.go` → `buildFilterChain()`.

## Architecture

```
external AP server
       │
       ▼
   HAProxy / nginx
       │
       ├── /api/* ──────────► Hollo (stream-proxy)
       ├── /inbox ──────────► inbox-guard ─► Hollo (filtered)
       └── /* ──────────────► Hollo (direct)
```

## Horizontal scaling

inbox-guard is fully stateless — all filtering is done per-request with no shared state. You can run multiple replicas behind a load balancer:

```yaml
# docker-compose.yml (multiple replicas)
services:
  inbox-guard:
    build: ./inbox-guard
    deploy:
      replicas: 3
    environment:
      - BACKEND=http://hollo:3000
      - ACTION=soft
```

All instances receive identical configuration via environment variables. Use the `/metrics` endpoint for autoscaling decisions and `/health` for liveness probes.

## License

AGPL-3.0 (same as Hollo)
