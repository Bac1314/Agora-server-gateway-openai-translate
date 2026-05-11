#!/usr/bin/env bash
set -euo pipefail

# ── Translator Bot — build and run ─────────────────────────────────────────────
#
# Usage:
#   OPENAI_API_KEY=sk-...  AGORA_APP_ID=<id>  ./run.sh [--channel <name>] [extra args]
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
#
# Listeners should subscribe to BOT_UID to hear translated audio.

: "${OPENAI_API_KEY:?OPENAI_API_KEY must be set}"
: "${AGORA_APP_ID:?AGORA_APP_ID must be set}"

CHANNEL="${CHANNEL:-translate-test}"
SPEAKER_UID="${SPEAKER_UID:-0}"
BOT_UID="${BOT_UID:-2002}"
SRC_LANG="${SRC_LANG:-en}"
DST_LANG="${DST_LANG:-es}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "[run.sh] Building Docker image (platform: linux/arm64)..."
docker build \
    --platform linux/arm64 \
    -t translator-bot \
    "$SCRIPT_DIR"

echo ""
echo "[run.sh] Starting translator bot"
echo "  Channel  : $CHANNEL"
echo "  Speaker  : $SPEAKER_UID (0 = all users)"
echo "  Bot      : $BOT_UID  ← listeners subscribe to this UID"
echo "  Language : $SRC_LANG → $DST_LANG"
echo ""

docker run --rm \
    --platform linux/arm64 \
    -e OPENAI_API_KEY="$OPENAI_API_KEY" \
    translator-bot \
    --token "$AGORA_APP_ID" \
    --channelId "$CHANNEL" \
    --speakerUid "$SPEAKER_UID" \
    --botUid "$BOT_UID" \
    --srcLang "$SRC_LANG" \
    --dstLang "$DST_LANG" \
    "$@"
