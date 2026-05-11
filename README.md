# Agora MiddleMan — OpenAI Realtime Translation PoC

A server-side translator bot that joins an Agora RTC channel, subscribes to a speaker's PCM audio, bridges it through OpenAI's `gpt-realtime-translate` via WebSocket, and republishes translated PCM under a bot UID. Listeners subscribe to that UID to hear real-time translated audio.

**Key advantage:** One OpenAI session serves all listeners regardless of count — translation cost stays flat while fanout scales to thousands of listeners via Agora SD-RTN.

```
Speaker (Web/iOS/Android)
  └─▶ Agora RTC Channel
        └─▶ translator-bot (Linux process)
              ├─▶ OpenAI gpt-realtime-translate (WebSocket)
              └─▶ publishes translated audio as bot UID
                    └─▶ Listeners (×N) — subscribe to bot UID only
```

## Cost comparison (1 hr, 500 listeners, 1 language pair)

| Approach | Translation cost | Total (incl. Agora RTC) |
|---|---|---|
| **This PoC** (Server Gateway + OpenAI) | ~$2.04 | **~$31.91** |
| Agora ConvoAI | ~$6.00 | ~$35.77 |
| OpenAI direct (no fanout) | ~$1,020 | ~$1,020+ |

See [`Architecture_Proposal.md`](.claude/Architecture_Proposal.md) for full analysis.

## Prerequisites

- **Docker Desktop** — Apple Silicon (arm64 native; no emulation needed)
- **Agora App ID** — from [console.agora.io](https://console.agora.io); token auth must be **disabled**
- **OpenAI API key** — with Realtime API access
- **Agora Linux SDK v4.4.32** — aarch64 build, placed at `agora_rtc_sdk/agora_sdk/`

### Download the Agora Linux SDK

The `.so` binaries are not included in this repo. Download SDK v4.4.32 (aarch64) from the [Agora Downloads page](https://docs.agora.io/en/sdks) and place the files:

```
agora_rtc_sdk/agora_sdk/
  include/           ← SDK headers
  libagora_rtc_sdk.so
  libagora-fdkaac.so
  libaosl.so
```

## Quick Start

```bash
export OPENAI_API_KEY=sk-...
export AGORA_APP_ID=<your-agora-app-id>
export DST_LANG=ja       # target language (ja, es, fr, zh, de, ...)
./run.sh
```

`run.sh` builds the Docker image then starts the bot. First build ~5 min (compiles SDK examples, downloads `nlohmann/json`).

### Environment variables

| Variable | Default | Description |
|---|---|---|
| `OPENAI_API_KEY` | required | OpenAI key with Realtime access |
| `AGORA_APP_ID` | required | Agora App ID |
| `CHANNEL` | `translate-test` | Channel name |
| `SPEAKER_UID` | `0` | UID to translate (`0` = all users) |
| `BOT_UID` | `2002` | Bot UID — listeners subscribe here |
| `SRC_LANG` | `en` | Source language code |
| `DST_LANG` | `es` | Target language code |

## Testing with the Agora Web Demo

1. Open [webdemo.agora.io](https://webdemo.agora.io) in **two tabs**
2. **Tab 1** — join channel `translate-test`, UID `1001`, **host** role → speak
3. **Tab 2** — join channel `translate-test`, UID `3000` → subscribe to **UID `2002`** for translated audio

## Audio Pipeline

```
Agora RX  (16 kHz PCM16 mono)
  → Resampler 16k → 24k
  → base64 → OpenAI WS (input_audio_buffer.append)
  → response.audio.delta → base64 decode
  → Resampler 24k → 16k
  → JitterBuffer (3 s, silence on underrun)
  → 10 ms frame slicer
  → Agora TX (sendAudioPcmData, 160 samples/frame @ 16 kHz)
```

## Project Layout

```
agora_rtc_sdk/
  agora_sdk/                  Agora Linux SDK (.so + headers) — not in repo
  example/
    translator_bot/           ← all new code
      translator_bot.cpp      main: IRtcConnection + sender thread
      openai_ws_client.h/cpp  OpenAI Realtime WS (libwebsockets)
      audio_pipeline.h/cpp    Resampler (libsamplerate) + JitterBuffer
      CMakeLists.txt
    common/                   shared Agora helpers
Dockerfile                    arm64/Ubuntu 22.04 build+run image
run.sh                        one-command build+run wrapper
```

## Key Technical Constraints

- Channel profile must be `CHANNEL_PROFILE_LIVE_BROADCASTING` (Server Gateway requirement)
- `sendAudioPcmData` requires exactly **10 ms frames** (160 samples at 16 kHz)
- Supported PCM rates: 16 kHz or 48 kHz — **not 44.1 kHz**
- Single `IRtcConnection` subscribes to speaker audio and publishes translated audio simultaneously under its own UID
- OpenAI audio format: PCM16 24 kHz mono, base64-encoded over WebSocket

## Dependencies (installed in Dockerfile)

| Package | Purpose |
|---|---|
| `libwebsockets-dev` | WebSocket + TLS to OpenAI |
| `libsamplerate0-dev` | PCM resampling (16k↔24k) |
| `libssl-dev` | TLS (pulled by libwebsockets) |
| `nlohmann/json v3.11.3` | JSON parsing (downloaded as single header) |

## Known Open Items

1. **OpenAI model slug** — verify `gpt-realtime-translate` is the current model name and confirm exact `session.update` field names against [OpenAI Realtime Translation docs](https://platform.openai.com/docs/guides/realtime-translation).
2. **$0.034/min billing** — confirm whether input-only or combined input+output via OpenAI dashboard on first test call.
3. **Channels per host** — load test needed; estimate 10–50 per 8-core box.

## License

MIT
