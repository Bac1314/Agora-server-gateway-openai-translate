# REST Control Plane — Session Handoff

**Date:** 2026-05-12  
**Status:** Tasks 1–6 complete. Tasks 7–9 remaining.

---

## Where we are

Working in isolated git worktree:
```
/Users/bachuang/Downloads/!AllDemos/!_PoCs/Agora-server-gateway-openai-translate/.claude/worktrees/feat+rest-control-plane
Branch: worktree-feat+rest-control-plane
```

All Go server code is done. 18/18 tests pass under `go test -race ./cmd/server/`. Three infrastructure tasks remain.

---

## Completed (Tasks 1–6)

| Task | Commit | Description |
|---|---|---|
| 1 | 35c2879 | Go module + skeleton (go.mod, main.go, store.go, handlers.go, main_test.go) |
| 1 fix | ea48c15 | Stop stub → nil; explicit encode error discard |
| 2 | 3bd42b4 | Session store: Create, Stop, Get, List, watchProcess, resolveUID |
| 2 fix | c4001df | Get/List return value copies (race fix); ExitCode nil on non-ExitError; test cleanup |
| 3 | 5a4a9cf | TestHealth + TestAuthRequired integration tests |
| 4 | 9d62265 | POST /sessions handler |
| 4 fix | 91ce367 | Copy session before encoding (race fix) |
| 5 | 7378b71 | GET /sessions + GET /sessions/:id handlers |
| 6 | 9ed5680 | DELETE /sessions/:id handler |

Latest commit: `9ed5680`

### Key API decisions baked into code

- `store.Get()` → `(Session, bool)` value copy (race-safe)
- `store.List()` → `[]Session` value slice (race-safe)
- `store.Create()` → `(*Session, error)` — handler does `snapshot := *sess` before encoding
- Auth guards ALL routes including `/health` (spec-correct; load balancers must pass API key)
- `truePath(t)` helper in tests uses `exec.LookPath("true")` for macOS/Linux compat

---

## Remaining (Tasks 7–9)

### Task 7: Dockerfile update

Full spec in plan file. Summary:

Add `FROM golang:1.22 AS go-builder` stage BEFORE the existing Ubuntu stage:
```dockerfile
# ── Stage 1: Build Go server ───────────────────────────────────────────────────
FROM golang:1.22 AS go-builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY cmd/ ./cmd/
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server/

# ── Stage 2: Build C++ bot + runtime (existing content, keep all apt/cmake/build.sh lines) ──
FROM ubuntu:22.04
...existing apt + agora SDK download + nlohmann/json + build.sh...
```

At the END of the Ubuntu stage, replace the last few lines:
```dockerfile
# REMOVE these lines:
COPY docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh
WORKDIR /app/agora_rtc_sdk/example/out
ENTRYPOINT ["/app/docker-entrypoint.sh"]

# ADD these lines:
COPY --from=go-builder /server /app/server
ENV LD_LIBRARY_PATH=/app/agora_rtc_sdk/agora_sdk
ENV BOT_BINARY=/app/agora_rtc_sdk/example/out/translator_bot
EXPOSE 8080
WORKDIR /app
ENTRYPOINT ["/app/server"]
```

Note: Keep `ENV LD_LIBRARY_PATH=...` line that's already in the Dockerfile — just move it to the end section. Do NOT delete `docker-entrypoint.sh` from repo.

Commit: `feat: add Go build stage to Dockerfile, change ENTRYPOINT to REST server`

---

### Task 8: render.yaml + CloudFormation

**render.yaml** — full replacement:
```yaml
services:
  - type: web
    name: agora-translator-bot
    runtime: image
    image:
      url: ghcr.io/bac1314/agora-server-gateway-openai-translate:latest
    envVars:
      - key: API_KEY
        sync: false
      - key: PORT
        value: "8080"
      - key: MAX_SESSIONS
        value: "10"
      - key: BOT_BINARY
        value: /app/agora_rtc_sdk/example/out/translator_bot
```
Changes: `worker` → `web`, replace all old env vars with new server env vars.

**deploy/aws/translator-bot.cfn.yaml** — three surgical edits:

1. Add `ApiKey` parameter (after `BotUid` parameter block):
```yaml
  ApiKey:
    Type: String
    NoEcho: true
    Description: Static API key for X-Api-Key header authentication
```

2. In the ECS task container definition, replace the `Environment` section:
```yaml
              Environment:
                - Name: API_KEY
                  Value: !Ref ApiKey
                - Name: MAX_SESSIONS
                  Value: "10"
                - Name: PORT
                  Value: "8080"
```
(Remove old `AGORA_APP_ID`, `OPENAI_API_KEY`, `CHANNEL`, `SRC_LANG`, `DST_LANG`, `SPEAKER_UID`, `BOT_UID`, `IDLE_EXIT_SECONDS` env entries — these are now per-session in POST body.)

3. Add port 8080 to the security group ingress and container port mappings:
```yaml
# In SecurityGroup ingress:
      SecurityGroupIngress:
        - IpProtocol: tcp
          FromPort: 8080
          ToPort: 8080
          CidrIp: 0.0.0.0/0

# In ContainerDefinitions PortMappings:
              PortMappings:
                - ContainerPort: 8080
                  Protocol: tcp
```

Commit: `feat: update Render + CFN configs for REST server (port 8080, API_KEY)`

---

### Task 9: CLAUDE.md + README.md

**CLAUDE.md** changes:
1. Add `cmd/server/` entries to "Project layout" section
2. Replace "How to build and run" → REST server run instructions + REST API table
3. Add "Server environment variables" table (API_KEY, PORT, MAX_SESSIONS, BOT_BINARY)

**README.md** changes:
1. Replace "Environment variables / CLI flags" table with REST quick-start section
2. Add curl examples for POST /sessions and DELETE /sessions/:id
3. Add server env vars table

Full wording is in the plan: `docs/superpowers/plans/2026-05-11-rest-control-plane.md` Task 9.

Commit: `docs: update CLAUDE.md + README.md for REST control plane`

---

## How to resume

1. Open this file for context
2. Switch into the worktree:
   ```
   Use EnterWorktree with path: /Users/bachuang/Downloads/!AllDemos/!_PoCs/Agora-server-gateway-openai-translate/.claude/worktrees/feat+rest-control-plane
   ```
3. Invoke `superpowers:subagent-driven-development`
4. Tell Claude: "Resuming REST control plane implementation. Worktree already set up. Tasks 1–6 done (18/18 tests passing). Resume at Task 7 (Dockerfile). Handoff doc at docs/superpowers/specs/2026-05-12-rest-control-plane-handoff.md. Plan at docs/superpowers/plans/2026-05-11-rest-control-plane.md."
5. Dispatch Task 7 implementer, then Task 8, then Task 9
6. After all done, invoke `superpowers:finishing-a-development-branch`

---

## Verification (run after Tasks 7–9)

```bash
# All Go tests still pass
go test -race ./cmd/server/ -v

# Docker builds (full build ~5 min)
docker build --platform linux/arm64 -t translator-bot-server .

# Smoke test
docker run --rm -e API_KEY=test -p 8080:8080 -d translator-bot-server
curl -s -H "X-Api-Key: test" http://localhost:8080/health
# → {"sessions":0,"status":"ok"}
```
