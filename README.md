# inbox-guard

ActivityPub inbox spam filter for Mastodon, Misskey, and any ActivityPub-compatible server.

Runs as a reverse proxy in front of `/inbox`. Filters messages before they reach your ActivityPub server.

## Features

- **Mention spam detection** — blocks posts with excessive `@mentions` (configurable threshold)
- **Targeted mention filtering** — optionally only enforce the mention filter when a post actually mentions your account or replies to one of your posts (avoids blocking unrelated group/broadcast mentions)
- **Content ratio check** — blocks posts where >90% of content is mentions (no real content)
- **Keyword/URL block** — blocks posts containing specified keywords or URLs
- **Domain block** — blocks posts from specified domains
- **Silent drop (soft)** or **explicit reject (block)** modes
- **Zero dependencies** — standard library only, ~5MB Docker image (from scratch)
- **Health check** endpoint at `/health`
- **Prometheus-compatible metrics** at `/metrics`

## Quick start

```yaml
# docker-compose.yml
services:
  inbox-guard:
    image: ghcr.io/ntsklab/inbox-guard:latest
    restart: always
    ports:
      - "3000:3000"
    environment:
      - BACKEND=http://your-ap-server:3000
      - ACTION=200
      - MAX_MENTIONS=5
      - BLOCK_KEYWORDS=bad-invite.example.com,spam-strings
      - BLOCK_DOMAINS=spam-server.example.com
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:3000/health"]
      interval: 30s
      timeout: 5s
      retries: 3
```

Then route `/inbox` traffic to inbox-guard.

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `BACKEND` | **(required)** | Backend server URL to proxy to |
| `LISTEN_PORT` | `3000` | Port to listen on |
| `ACTION` | `403` | HTTP status code to return when blocking (e.g. `403`, `200`) |
| `MAX_MENTIONS` | `4` | Maximum allowed @mentions in a post |
| `MAX_CONTENT_RATIO` | `0.9` | Max ratio of mention content to total content (0.0–1.0) |
| `MENTION_FILTER_TARGET` | `always` | When to enforce the mention filter. `always` (current behavior), `mentioned` (only if the post mentions your domain), `in_reply_to` (only if it replies to a post on your domain), or `mentioned_or_in_reply_to` |
| `LOCAL_DOMAIN` | (empty) | Your instance domain (e.g. `instance.example`). Required for `MENTION_FILTER_TARGET` other than `always`. If unset, the mention filter always runs |
| `BLOCK_KEYWORDS` | (empty) | Comma-separated keywords/URLs |
| `BLOCK_DOMAINS` | (empty) | Comma-separated domains to block |
| `LOG_LEVEL` | `info` | `info` or `debug` |
| `READ_TIMEOUT` | `10s` | Max duration for reading the entire request |
| `WRITE_TIMEOUT` | `30s` | Max duration before timing out writes |
| `IDLE_TIMEOUT` | `60s` | Max time to wait for next request on a keep-alive connection |
| `SHUTDOWN_TIMEOUT` | `30s` | Max time to wait for in-flight requests during graceful shutdown |

## Targeted mention filtering

By default (`MENTION_FILTER_TARGET=always`) the mention filter blocks any post whose mention
count or ratio exceeds the thresholds, regardless of who it is addressed to. On a busy
federated timeline this can reject posts that merely broadcast many mentions to remote users
but never mention anyone on your instance.

To only enforce the mention filter when a post is actually aimed at your instance, set:

```
MENTION_FILTER_TARGET=mentioned_or_in_reply_to
LOCAL_DOMAIN=instance.example
```

`LOCAL_DOMAIN` matching is used because ActivityPub implementations represent mentions
differently (Mastodon HTML `h-card`/`u-url mention`, Misskey `mention`, AP `tag` entries,
and plain-text `@user@domain`). The check covers all of them, plus the activity `to`/`cc`
recipient fields, which is the most reliable "this is addressed to us" signal across servers.

Modes:

| Mode | Mention filter applies when |
|---|---|
| `always` | always (default, legacy behavior) |
| `mentioned` | the post mentions a user on `LOCAL_DOMAIN` |
| `in_reply_to` | the post's `inReplyTo` points to a post on `LOCAL_DOMAIN` |
| `mentioned_or_in_reply_to` | either of the above |

If `MENTION_FILTER_TARGET` is not `always` but `LOCAL_DOMAIN` is unset, inbox-guard logs a
warning and falls back to `always` behavior.

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
   reverse proxy (nginx / HAProxy)
       │
       ├── /inbox ──────────► inbox-guard ─► your AP server
       └── /* ──────────────► your AP server
```
