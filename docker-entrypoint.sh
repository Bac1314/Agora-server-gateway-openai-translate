#!/usr/bin/env bash
set -euo pipefail

# Bridge env vars → translator_bot CLI flags.
# All vars have defaults matching the bot's built-in defaults.

AGORA_APP_ID="${AGORA_APP_ID:?AGORA_APP_ID env var must be set}"
CHANNEL="${CHANNEL:-translate-test}"
SPEAKER_UID="${SPEAKER_UID:-0}"
BOT_UID="${BOT_UID:-2002}"
SRC_LANG="${SRC_LANG:-en}"
DST_LANG="${DST_LANG:-es}"
IDLE_EXIT_SECONDS="${IDLE_EXIT_SECONDS:-300}"

exec /app/agora_rtc_sdk/example/out/translator_bot \
    --token          "$AGORA_APP_ID" \
    --channelId      "$CHANNEL" \
    --speakerUid     "$SPEAKER_UID" \
    --botUid         "$BOT_UID" \
    --srcLang        "$SRC_LANG" \
    --dstLang        "$DST_LANG" \
    --idleExitSeconds "$IDLE_EXIT_SECONDS" \
    "$@"
