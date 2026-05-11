#!/usr/bin/env bash
set -euo pipefail

# ── Translator Bot — build and run ─────────────────────────────────────────────
#
# Usage:
#   OPENAI_API_KEY=sk-...  AGORA_APP_ID=<id>  ./run.sh [extra args]
#   OPENAI_API_KEY=sk-...  AGORA_APP_ID=<id>  ./run.sh --pull  [extra args]
#
# Required env vars:
#   OPENAI_API_KEY   — OpenAI API key with Realtime access
#   AGORA_APP_ID     — Agora App ID (from console.agora.io); token auth must be
#                      DISABLED for this project in the Agora Console.
#
# Optional env vars / flags:
#   CHANNEL          — Channel name (default: translate-test)
#   SPEAKER_UID      — UID of the speaker to translate (default: 0 = all users)
#   BOT_UID          — UID the bot joins and publishes as (default: 2002)
#   SRC_LANG         — Source language code (default: en)
#   DST_LANG         — Target language code (default: es)
#   IDLE_EXIT_SECONDS — Exit after N seconds of audio silence (default: 300; 0 = disabled)
#
# Listeners should subscribe to BOT_UID to hear translated audio.

: "${OPENAI_API_KEY:?OPENAI_API_KEY must be set}"
: "${AGORA_APP_ID:?AGORA_APP_ID must be set}"

CHANNEL="${CHANNEL:-translate-test}"
SPEAKER_UID="${SPEAKER_UID:-0}"
BOT_UID="${BOT_UID:-2002}"
SRC_LANG="${SRC_LANG:-en}"
DST_LANG="${DST_LANG:-es}"
IDLE_EXIT_SECONDS="${IDLE_EXIT_SECONDS:-300}"

# Detect host architecture
case "$(uname -m)" in
    arm64|aarch64) PLATFORM="linux/arm64" ;;
    *)             PLATFORM="linux/amd64" ;;
esac

# --pull: skip local build, use prebuilt GHCR image
if [[ "${1:-}" == "--pull" ]]; then
    shift
    IMAGE="ghcr.io/bac1314/agora-server-gateway-openai-translate:latest"
    echo "[run.sh] Using prebuilt image: $IMAGE"
    docker run --rm \
        --platform "$PLATFORM" \
        -e OPENAI_API_KEY="$OPENAI_API_KEY" \
        -e AGORA_APP_ID="$AGORA_APP_ID" \
        -e CHANNEL="$CHANNEL" \
        -e SPEAKER_UID="$SPEAKER_UID" \
        -e BOT_UID="$BOT_UID" \
        -e SRC_LANG="$SRC_LANG" \
        -e DST_LANG="$DST_LANG" \
        -e IDLE_EXIT_SECONDS="$IDLE_EXIT_SECONDS" \
        "$IMAGE" \
        "$@"
    exit 0
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "[run.sh] Building Docker image (platform: $PLATFORM)..."
docker build \
    --platform "$PLATFORM" \
    -t translator-bot \
    "$SCRIPT_DIR"

echo ""
echo "[run.sh] Starting translator bot"
echo "  Channel  : $CHANNEL"
echo "  Speaker  : $SPEAKER_UID (0 = all users)"
echo "  Bot      : $BOT_UID  ← listeners subscribe to this UID"
echo "  Language : $SRC_LANG → $DST_LANG"
echo "  Idle exit: ${IDLE_EXIT_SECONDS}s of silence"
echo ""

docker run --rm \
    --platform "$PLATFORM" \
    -e OPENAI_API_KEY="$OPENAI_API_KEY" \
    -e AGORA_APP_ID="$AGORA_APP_ID" \
    -e CHANNEL="$CHANNEL" \
    -e SPEAKER_UID="$SPEAKER_UID" \
    -e BOT_UID="$BOT_UID" \
    -e SRC_LANG="$SRC_LANG" \
    -e DST_LANG="$DST_LANG" \
    -e IDLE_EXIT_SECONDS="$IDLE_EXIT_SECONDS" \
    translator-bot \
    "$@"
