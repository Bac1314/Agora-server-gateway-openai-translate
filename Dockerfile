FROM arm64v8/ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    build-essential \
    cmake \
    pkg-config \
    curl \
    libwebsockets-dev \
    libssl-dev \
    libsamplerate0-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy SDK and example sources into the image
COPY agora_rtc_sdk /app/agora_rtc_sdk

# Download nlohmann/json (used by translator_bot for OpenAI JSON parsing)
RUN mkdir -p /app/agora_rtc_sdk/example/third-party/json_parser/include && \
    curl -fsSL \
    "https://github.com/nlohmann/json/releases/download/v3.11.3/json.hpp" \
    -o /app/agora_rtc_sdk/example/third-party/json_parser/include/json.hpp

# Build all examples including translator_bot
WORKDIR /app/agora_rtc_sdk/example
RUN ./build.sh

# Agora SDK .so files are in agora_sdk/ relative to the example output
ENV LD_LIBRARY_PATH=/app/agora_rtc_sdk/agora_sdk

WORKDIR /app/agora_rtc_sdk/example/out

ENTRYPOINT ["/app/agora_rtc_sdk/example/out/translator_bot"]
