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

## Environment variables / CLI flags

| Env var | CLI flag | Default | Description |
|---|---|---|---|
| `AGORA_APP_ID` | `--token` | (required) | Agora App ID |
| — | — | — | `OPENAI_API_KEY` env var (required, no CLI flag) |
| `CHANNEL` | `--channelId` | `translate-test` | Channel to join |
| `SPEAKER_UID` | `--speakerUid` | `0` | UID to translate (0 = all) |
| `BOT_UID` | `--botUid` | `2002` | Bot's UID; listeners subscribe here |
| `SRC_LANG` | `--srcLang` | `en` | Source language |
| `DST_LANG` | `--dstLang` | `es` | Target language |
| `IDLE_EXIT_SECONDS` | `--idleExitSeconds` | `300` | Seconds of silence before auto-exit (0 = off) |

---

## Local development (build from source)

Requires Docker Desktop and Apple Silicon (arm64) or an amd64 Linux box.

```bash
export OPENAI_API_KEY=sk-...
export AGORA_APP_ID=<your-app-id>
export DST_LANG=ja  # target language

# Build and run
./run.sh

# Or use the prebuilt image (no build step)
./run.sh --pull
```

**Note:** The Agora Linux SDK is not bundled in this repo due to redistribution constraints.
For local `./run.sh` builds without the `--pull` flag, the SDK is downloaded at Docker build time from the GitHub Release assets.

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
