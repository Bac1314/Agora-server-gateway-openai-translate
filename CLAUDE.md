# Agora MiddleMan — OpenAI Realtime Translation PoC

## What this is

Translator bot: Linux process that joins an Agora RTC channel, subscribes to one speaker's PCM audio, bridges it through OpenAI `gpt-realtime-translate` via WebSocket, and republishes translated PCM under its bot UID. Listeners subscribe to that UID to hear translated audio. Also relays source-language and translated-language transcript text on an Agora data stream, JSON-encoded as `{lang,text,isFinal,ts}`.

No Agora ConvoAI. No client app — use https://webdemo.agora.io for speaker/listener testing.

## Project layout

```
agora_rtc_sdk/              Agora Linux SDK (aarch64, v4.4.32)
  agora_sdk/                .so files + headers
  example/
    translator_bot/         ← all new code lives here
      translator_bot.cpp    main: single broadcaster IRtcConnection + sender thread
      openai_ws_client.h/cpp  OpenAI Realtime WS (libwebsockets)
      audio_pipeline.h/cpp    Resampler (libsamplerate) + JitterBuffer
      CMakeLists.txt
    common/                 shared Agora helpers (read-only)
cmd/
  server/
    main.go         HTTP server entry point, auth middleware, /health
    store.go        Session store (in-memory map), subprocess lifecycle
    handlers.go     HTTP handler functions
    main_test.go    Integration tests (httptest + fake-bot subprocess)
go.mod              Go module (stdlib only, no external deps)
Dockerfile                  arm64/Ubuntu 22.04 build+run image
run.sh                      one-command build+run wrapper
Architecture_Proposal.md   cost/scalability/comparison analysis
```

## How to build and run

### Prerequisites
- Docker Desktop with Apple Silicon (arm64 native — no emulation)
- Agora Console test project with **token auth disabled**
- OpenAI API key with Realtime access

### Run (REST server mode)

```bash
docker build --platform linux/arm64 -t translator-bot-server .
docker run --rm \
  -e API_KEY=my-secret-key \
  -p 8080:8080 \
  translator-bot-server
```

### REST API

All requests require `X-Api-Key: <API_KEY>` header.

| Method | Path | Description |
|---|---|---|
| POST | /sessions | Start a translator bot |
| GET | /sessions | List running sessions |
| GET | /sessions/:id | Get session status + exitCode |
| DELETE | /sessions/:id | Stop bot (SIGTERM) |
| GET | /health | Liveness check |

**POST /sessions body:**
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
`botUid` is optional — omit to auto-assign from pool 2000–2999.

### Server environment variables

| Var | Default | Description |
|---|---|---|
| `API_KEY` | (required) | Static key for `X-Api-Key` auth |
| `PORT` | `8080` | HTTP listen port |
| `MAX_SESSIONS` | `10` | Hard cap on concurrent bot processes (returns 503 when full) |
| `BOT_BINARY` | `/app/agora_rtc_sdk/example/out/translator_bot` | Path to bot binary |

### Test with web demo

1. Open https://webdemo.agora.io in two tabs
2. Tab 1: join channel `translate-test`, UID `1001`, host role — speak
3. Tab 2: join channel `translate-test`, UID `3000` — subscribe to the `botUid` returned by POST /sessions (auto-assigned from 2000–2999; pass `"botUid": 2002` in the request body to pin it)

## Audio pipeline

```
Agora RX (16kHz PCM16 mono)
  → Resampler 16k→24k
  → base64 → OpenAI WS (input_audio_buffer.append)
  → response.audio.delta → base64 decode
  → Resampler 24k→16k
  → JitterBuffer (3s, silence on underrun)
  → 10ms frame slicer
  → Agora TX (sendAudioPcmData, 160 samples/frame)
```

## Transcript pipeline

```
OpenAI session.{input,output}_transcript.{delta,done}
  → TranslatorBot TranscriptCallback
  → JSON {lang, text, isFinal, ts}
  → IRtcConnection::sendStreamMessage (reliable + ordered)
  → Listeners: ILocalUserObserver::onStreamMessage
```

- `lang` — ISO language code (`srcLang` for input, `dstLang` for output)
- `isFinal=false` on delta events, `isFinal=true` on done/completed events
- `ts` — ms since Unix epoch at send time

## Key technical constraints

- Server Gateway requires `CHANNEL_PROFILE_LIVE_BROADCASTING` mode
- `sendAudioPcmData` requires exactly **10ms frames** (160 samples at 16kHz)
- PCM must be 16kHz or 48kHz — **not 44.1kHz**
- Single broadcaster `IRtcConnection` that simultaneously subscribes to the speaker's audio and publishes translated PCM under its own UID
- OpenAI audio format: PCM16 24kHz mono, base64-encoded over WebSocket
- Agora data stream: 30 packets/s, 1 KB/packet, 6 KB/s, 5 streams/user (`NGIAgoraRtcConnection.h:428-430`)

## Known open items

1. **OpenAI model slug** — `gpt-realtime-translate` needs verification. See comment in `openai_ws_client.cpp` around `eventLoop()`. Check https://platform.openai.com/docs/guides/realtime-translation for current model name and exact `session.update` field names.
2. **`$0.034/min` billing direction** — confirm input-only vs combined via OpenAI dashboard during first test call.
3. **Channels per host** — load test needed; estimate 10–50 per 8-core box.

## Dependencies (installed in Dockerfile)

- `libwebsockets-dev` — WebSocket + TLS
- `libsamplerate0-dev` — PCM resampling
- `libssl-dev` — TLS (pulled by libwebsockets)
- `nlohmann/json v3.11.3` — JSON parsing (downloaded as single header)
