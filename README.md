# Agora Translator Bot

Real-time audio translation PoC: joins an Agora RTC channel, subscribes to a speaker's audio, translates it via OpenAI Realtime API, and republishes translated audio under a bot UID. Listeners subscribe to that UID.

## Pick your path

| | Platform | What you need |
|---|---|---|
| [![Deploy to Render](https://render.com/images/deploy-to-render-button.svg)](https://render.com/deploy?repo=https://github.com/Bac1314/Agora-server-gateway-openai-translate) | **Render** (amd64) | Render account, Agora App ID, OpenAI key |
| [![Launch Stack](https://s3.amazonaws.com/cloudformation-examples/cloudformation-launch-stack.png)](https://console.aws.amazon.com/cloudformation/home#/stacks/create/review?templateURL=https://raw.githubusercontent.com/Bac1314/Agora-server-gateway-openai-translate/main/deploy/aws/translator-bot.cfn.yaml&stackName=translator-bot) | **AWS Fargate** (ECS) | AWS account, Agora App ID, OpenAI key |
| `docker run -e OPENAI_API_KEY=... -e AGORA_APP_ID=... ghcr.io/bac1314/agora-server-gateway-openai-translate:latest` | **Local Docker** (one-liner) | Docker Desktop, Agora App ID, OpenAI key |

> **Prerequisite for all paths:** Disable token authentication in the [Agora Console](https://console.agora.io) for your project.
> Settings → Project Management → (your project) → Security → Primary certificate → toggle **OFF**.

---

## Test the translation

Open the tester pages to speak and hear translation in your browser:

- **Speaker**: https://bac1314.github.io/Agora-server-gateway-openai-translate/speaker.html
- **Listener**: https://bac1314.github.io/Agora-server-gateway-openai-translate/listener.html

Or start from the [landing page](https://bac1314.github.io/Agora-server-gateway-openai-translate/) to configure channel + App ID together.

**Quick test flow:**
1. Start the bot (any path above)
2. Open Speaker in one tab — enter your Agora App ID, join channel `translate-test`
3. Open Listener in another tab (or share the link) — click "Start Listening"
4. Speak in the Speaker tab → hear translation in Listener tab + see captions in both

---

## Quick start — REST API

```bash
docker build --platform linux/arm64 -t translator-bot-server .
docker run --rm -e API_KEY=my-secret-key -p 8080:8080 translator-bot-server
```

Start a translation session:

```bash
curl -s -X POST http://localhost:8080/sessions \
  -H "X-Api-Key: my-secret-key" \
  -H "Content-Type: application/json" \
  -d '{
    "agoraAppId":  "<your-agora-app-id>",
    "openAiKey":   "sk-...",
    "channel":     "translate-test",
    "srcLang":     "en",
    "dstLang":     "es"
  }'
```

Returns `{"sessionId":"...","botUid":2000,"channel":"translate-test","status":"running",...}`.

Listeners subscribe to `botUid` (auto-assigned from 2000–2999 if omitted).

Stop a session:

```bash
curl -s -X DELETE http://localhost:8080/sessions/<sessionId> \
  -H "X-Api-Key: my-secret-key"
```

## Server environment variables

| Var | Default | Description |
|---|---|---|
| `API_KEY` | (required) | Static key for `X-Api-Key` auth |
| `PORT` | `8080` | HTTP listen port |
| `MAX_SESSIONS` | `10` | Hard cap; returns 503 when full |
| `BOT_BINARY` | `/app/agora_rtc_sdk/example/out/translator_bot` | Path to bot binary |

---

## Architecture

```
Speaker mic
  → Agora RTC channel
  → Bot subscribes (16kHz PCM)
  → Resampler 16k→24k
  → OpenAI Realtime API (gpt-realtime-translate)
  → Translated 24kHz PCM
  → Resampler 24k→16k
  → JitterBuffer
  → Bot publishes as UID 2002
  → Listeners subscribe to UID 2002
```

Transcript JSON `{lang, text, isFinal, ts}` sent on Agora data stream. Speaker + Listener web pages render live captions.

---

## Known issues / open items

1. **OpenAI model slug** — `gpt-realtime-translate` needs verification at [platform.openai.com/docs/guides/realtime-translation](https://platform.openai.com/docs/guides/realtime-translation).
2. **SDK redistribution** — Agora SDK tarballs in GitHub Release require license verification before broad distribution.
3. **Channels per host** — Load test needed; estimate 10–50 per 8-core box.
