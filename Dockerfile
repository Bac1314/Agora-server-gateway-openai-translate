# ── Stage 1: Build Go server ───────────────────────────────────────────────────
FROM golang:1.22 AS go-builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY cmd/ ./cmd/
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server/

# ── Stage 2: Build C++ bot + runtime image ────────────────────────────────────
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

RUN if [ ! -f /app/agora_rtc_sdk/agora_sdk/libagora_rtc_sdk.so ]; then \
        echo "Downloading Agora SDK ${AGORA_SDK_VERSION} for ${TARGETARCH}..." && \
        curl -fsSL \
            "https://github.com/Bac1314/Agora-server-gateway-openai-translate/releases/download/sdk-${AGORA_SDK_VERSION}/Agora_Native_SDK_${TARGETARCH}.tgz" \
            | tar xz -C /app/ \
            && echo "Agora SDK download complete"; \
    else \
        echo "Agora SDK already present (local-dev path)"; \
    fi

RUN mkdir -p /app/agora_rtc_sdk/example/third-party/json_parser/include && \
    curl -fsSL \
    "https://github.com/nlohmann/json/releases/download/v3.11.3/json.hpp" \
    -o /app/agora_rtc_sdk/example/third-party/json_parser/include/json.hpp

WORKDIR /app/agora_rtc_sdk/example
RUN ./build.sh

# Copy Go server binary from go-builder stage
COPY --from=go-builder /server /app/server

ENV LD_LIBRARY_PATH=/app/agora_rtc_sdk/agora_sdk
ENV BOT_BINARY=/app/agora_rtc_sdk/example/out/translator_bot

EXPOSE 8080
WORKDIR /app
ENTRYPOINT ["/app/server"]
