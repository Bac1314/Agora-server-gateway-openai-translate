FROM ubuntu:22.04

ARG TARGETARCH
ARG AGORA_SDK_VERSION=4.4.32

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

COPY agora_rtc_sdk /app/agora_rtc_sdk

# Download Agora SDK if not already present (CI path; local dev copies .so files directly)
RUN if [ ! -f /app/agora_rtc_sdk/agora_sdk/libagora_rtc_sdk.so ]; then \
        echo "Downloading Agora SDK ${AGORA_SDK_VERSION} for ${TARGETARCH}..." && \
        curl -fsSL \
            "https://github.com/Bac1314/Agora-server-gateway-openai-translate/releases/download/sdk-${AGORA_SDK_VERSION}/Agora_Native_SDK_${TARGETARCH}.tgz" \
            | tar xz -C /app/ \
            && echo "Agora SDK download complete"; \
    else \
        echo "Agora SDK already present (local-dev path)"; \
    fi

# Download nlohmann/json (used by translator_bot for OpenAI JSON parsing)
RUN mkdir -p /app/agora_rtc_sdk/example/third-party/json_parser/include && \
    curl -fsSL \
    "https://github.com/nlohmann/json/releases/download/v3.11.3/json.hpp" \
    -o /app/agora_rtc_sdk/example/third-party/json_parser/include/json.hpp

# Build all examples including translator_bot
WORKDIR /app/agora_rtc_sdk/example
RUN ./build.sh

COPY docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

# Agora SDK .so files are in agora_sdk/ relative to the example output
ENV LD_LIBRARY_PATH=/app/agora_rtc_sdk/agora_sdk

WORKDIR /app/agora_rtc_sdk/example/out

ENTRYPOINT ["/app/docker-entrypoint.sh"]
