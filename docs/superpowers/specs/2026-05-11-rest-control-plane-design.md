# REST Control Plane Design

**Status: COMPLETE — all decisions locked**

## What this is

Add an HTTP control plane so callers can dynamically start/stop translator bot sessions for arbitrary channels, instead of the current single-channel hardcoded daemon.

## All decisions locked

| Topic | Decision |
|---|---|
| Deploy model | Single Go server + subprocess per session (sidecar in same container) |
| Language | Go |
| Auth | Static API key via `X-Api-Key` header, set as env var on server |
| Credentials | Caller-supplied per session — `agoraAppId` + `openAiKey` in POST body (multi-tenant) |
| Session storage | In-memory map (`map[string]*Session`) — lost on restart, clean slate intentional |
| Bot UID assignment | Hybrid: caller specifies `botUid` or omits to auto-assign from pool 2000–2999 |
| Max concurrent sessions | Hard cap via `MAX_SESSIONS` env var, default 10, returns 503 when full |
| Session auto-cleanup | Goroutine per session blocks on `cmd.Wait()` — instant cleanup on crash or idle-exit |

## API

### Endpoints

```
POST   /sessions          Start bot for a channel
GET    /sessions          List all active sessions
GET    /sessions/:id      Get single session status
DELETE /sessions/:id      Stop bot (SIGTERM)
GET    /health            Liveness check
```

Auth: `X-Api-Key` header required on all routes. Returns `401` if missing or wrong.

### POST /sessions

**Request body:**
```json
{
  "agoraAppId":      "...",
  "openAiKey":       "sk-...",
  "channel":         "my-channel",
  "srcLang":         "en",
  "dstLang":         "es",
  "speakerUid":      0,
  "botUid":          2002,
  "idleExitSeconds": 300
}
```

- `botUid` — optional. Omit to auto-assign from pool 2000–2999.
- `speakerUid` — 0 = translate all users.
- `idleExitSeconds` — bot self-exits after this many seconds of silence.

**201 response:**
```json
{
  "sessionId":  "abc123",
  "botUid":     2002,
  "channel":    "my-channel",
  "startedAt":  "2026-05-11T10:00:00Z",
  "status":     "running"
}
```

### GET /sessions

Returns array of session objects (same shape as 201 response).

### GET /sessions/:id

Same shape as 201 response, plus:
```json
{
  "exitCode": null
}
```
`exitCode` is `null` while running, integer after process exits.

### DELETE /sessions/:id

Sends SIGTERM to bot process. Returns `204 No Content`. Returns `404` if not found.

### GET /health

```json
{ "status": "ok", "sessions": 3 }
```

### Error shape

All errors:
```json
{ "error": "description" }
```

### Status codes

| Code | When |
|---|---|
| 201 | Session started |
| 400 | Missing required fields |
| 401 | Missing or wrong API key |
| 404 | Session not found |
| 409 | Duplicate `channel` + `botUid` combination already running |
| 503 | `MAX_SESSIONS` limit reached |

## Architecture

```
HTTP Client
  → POST /sessions { agoraAppId, openAiKey, channel, ... }
  → Go server (port 8080, X-Api-Key auth)
  → os/exec.Command("translator_bot", --token, --channelId, ...)
     OPENAI_API_KEY set in subprocess env
  → goroutine blocks on cmd.Wait() → removes session from map on exit
  → DELETE /sessions/:id → SIGTERM → bot exits → goroutine cleans up

Go server + translator_bot binary coexist in same Docker image.
Server is new ENTRYPOINT. Bot binary at fixed path.
```

## Session lifecycle

```
POST /sessions
  → validate fields
  → check MAX_SESSIONS cap → 503 if full
  → resolve botUid (caller-supplied or auto-assign from pool)
  → check no duplicate channel+botUid → 409 if collision
  → os/exec.Start() translator_bot subprocess
  → add to in-memory map
  → spawn goroutine: cmd.Wait() → delete from map when done
  → return 201

DELETE /sessions/:id
  → lookup session → 404 if missing
  → cmd.Process.Signal(syscall.SIGTERM)
  → return 204 (goroutine handles map cleanup async)
```

## Bot UID pool

Auto-assign range: 2000–2999 (1000 slots). Server scans map for next unused UID in range. Returns 503 with `"error": "bot UID pool exhausted"` if all taken (only reachable if MAX_SESSIONS > 1000, which is unrealistic).

## Environment variables

| Var | Default | Description |
|---|---|---|
| `API_KEY` | (required) | Static key for `X-Api-Key` auth |
| `PORT` | `8080` | HTTP listen port |
| `MAX_SESSIONS` | `10` | Hard cap on concurrent bot processes |
| `BOT_BINARY` | `/app/agora_rtc_sdk/example/out/translator_bot` | Path to bot binary |

## Files changed vs current codebase

| File | Change |
|---|---|
| `cmd/server/main.go` | New: Go HTTP server |
| `Dockerfile` | Add Go build stage; change `ENTRYPOINT` to `server` binary |
| `docker-entrypoint.sh` | Retired (replaced by server subprocess logic) |
| `render.yaml` | Add `port: 8080`, change from `worker` to `web` service type |
| `deploy/aws/translator-bot.cfn.yaml` | Add ingress rule for port 8080, update task command |
| `CLAUDE.md` | Document REST control plane architecture + env vars |
| `README.md` | Document REST API endpoints + new run instructions |

## Key constraints inherited from bot

- `CHANNEL_PROFILE_LIVE_BROADCASTING` required by Server Gateway
- `sendAudioPcmData` requires exactly 10ms frames (160 samples at 16kHz)
- Agora data stream: 30 packets/s, 1 KB/packet, 5 streams/user max
