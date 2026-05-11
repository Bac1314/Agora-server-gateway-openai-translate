# REST Control Plane Design — Handoff

**Status: DESIGN IN PROGRESS — resume brainstorming to complete**

## What this is

Add an HTTP control plane so callers can dynamically start/stop translator bot sessions for arbitrary channels, instead of the current single-channel hardcoded daemon.

## Decisions locked

| Topic | Decision |
|---|---|
| Deploy model | Single Go server + subprocess per session (Option A — sidecar in same container) |
| Language | Go |
| Auth | Static API key via `X-Api-Key` header, set as env var on server |
| Credentials | Caller-supplied per session — `agoraAppId` + `openAiKey` in POST body (multi-tenant) |

## Decisions still needed (resume here)

1. **Session storage** — where to track active sessions + their PIDs? In-memory map (simplest, lost on restart) vs. file-based vs. SQLite?
2. **Bot UID assignment** — caller specifies `botUid`, or server auto-assigns from a pool to avoid collisions across concurrent sessions?
3. **Max concurrent sessions** — hard cap per server? (prevents OOM)
4. **Session auto-cleanup** — when bot process exits (idle-exit or crash), server needs to detect and clean up session record. How? (poll PIDs, or bot writes exit code to file)
5. **API shape approval** — present full endpoint design to user for sign-off before writing spec

## Proposed API shape (draft — not yet approved)

```
POST   /sessions          Start bot for a channel
GET    /sessions          List all active sessions
GET    /sessions/:id      Get single session status
DELETE /sessions/:id      Stop bot (SIGTERM)
GET    /health            Liveness check
```

**POST /sessions body:**
```json
{
  "agoraAppId":  "...",
  "openAiKey":   "sk-...",
  "channel":     "my-channel",
  "srcLang":     "en",
  "dstLang":     "es",
  "speakerUid":  "0",
  "botUid":      "2002",
  "idleExitSeconds": 300
}
```

**Response:**
```json
{
  "sessionId": "abc123",
  "botUid":    "2002",
  "channel":   "my-channel",
  "startedAt": "2026-05-11T10:00:00Z"
}
```

## Architecture (approved)

```
HTTP Client
  → POST /sessions { agoraAppId, openAiKey, channel, ... }
  → Go server (port 8080, X-Api-Key auth)
  → os/exec.Command("translator_bot", --token, --channelId, ...)
     with OPENAI_API_KEY set in process env
  → Bot process runs, joins Agora channel, translates
  → DELETE /sessions/:id → SIGTERM → bot exits cleanly

Go server + translator_bot binary coexist in same Docker image.
Server is new ENTRYPOINT; bot binary at fixed path.
```

## What changes vs current codebase

| File | Change |
|---|---|
| `Dockerfile` | Add Go build stage; change `ENTRYPOINT` from `docker-entrypoint.sh` to `server` binary |
| `docker-entrypoint.sh` | Retired (replaced by server's subprocess logic) |
| `render.yaml` | Add `port: 8080`, change from `worker` to `web` service type |
| `deploy/aws/translator-bot.cfn.yaml` | Add ingress rule for port 8080, update task command |
| `cmd/server/main.go` | New: Go HTTP server (to be created) |
| `README.md` | Document REST API |

## Bot binary path in container

`/app/agora_rtc_sdk/example/out/translator_bot`

## Resume instructions for next session

1. Open this file for context
2. Invoke `superpowers:brainstorming` skill
3. Tell Claude: "Resuming REST control plane design. Decisions locked per spec doc at `docs/superpowers/specs/2026-05-11-rest-control-plane-design.md`. Resume at 'Decisions still needed' — ask about session storage first."
4. Work through remaining 5 questions, get API shape approved
5. Write final spec, invoke `writing-plans` skill
