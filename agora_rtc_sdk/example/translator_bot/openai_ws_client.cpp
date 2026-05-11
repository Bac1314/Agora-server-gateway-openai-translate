#include "openai_ws_client.h"

#include <cstring>
#include <stdexcept>

#include <libwebsockets.h>
#include <json.hpp>

#include "common/log.h"

// ── Base64 helpers ────────────────────────────────────────────────────────────

static const char kB64[] =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

static std::string base64Encode(const uint8_t* d, size_t n) {
    std::string out;
    out.reserve((n + 2) / 3 * 4);
    for (size_t i = 0; i < n; i += 3) {
        uint32_t b = (uint32_t)d[i] << 16;
        if (i + 1 < n) b |= (uint32_t)d[i + 1] << 8;
        if (i + 2 < n) b |= d[i + 2];
        out += kB64[(b >> 18) & 63];
        out += kB64[(b >> 12) & 63];
        out += (i + 1 < n) ? kB64[(b >> 6) & 63] : '=';
        out += (i + 2 < n) ? kB64[b & 63]        : '=';
    }
    return out;
}

static std::vector<int16_t> base64DecodePcm(const std::string& s) {
    static int8_t T[256];
    static bool   init = false;
    if (!init) {
        memset(T, -1, sizeof(T));
        for (int i = 0; i < 64; ++i) T[(uint8_t)kB64[i]] = i;
        T[(uint8_t)'='] = 0;
        init = true;
    }
    std::vector<uint8_t> bytes;
    bytes.reserve(s.size() * 3 / 4);
    uint32_t buf = 0;
    int bits = 0;
    for (char c : s) {
        if (T[(uint8_t)c] < 0) continue;
        buf = (buf << 6) | T[(uint8_t)c];
        bits += 6;
        if (bits >= 8) {
            bits -= 8;
            bytes.push_back((buf >> bits) & 0xFF);
        }
    }
    size_t nSamples = bytes.size() / sizeof(int16_t);
    std::vector<int16_t> out(nSamples);
    memcpy(out.data(), bytes.data(), nSamples * sizeof(int16_t));
    return out;
}

// ── libwebsockets C callback ──────────────────────────────────────────────────

static int callback_openai(struct lws* wsi, enum lws_callback_reasons reason,
                            void* /*user*/, void* in, size_t len) {
    auto* self = (OpenAIWsClient*)lws_context_user(lws_get_context(wsi));
    if (!self) return 0;
    switch (reason) {
    case LWS_CALLBACK_CLIENT_APPEND_HANDSHAKE_HEADER: {
        unsigned char** p   = (unsigned char**)in;
        unsigned char*  end = *p + len;
        std::string auth    = "Bearer " + self->apiKey();
        if (lws_add_http_header_by_name(wsi,
                (const unsigned char*)"Authorization:",
                (const unsigned char*)auth.c_str(), (int)auth.size(), p, end))
            return -1;
        const char* safetyId = "translator-bot";
        if (lws_add_http_header_by_name(wsi,
                (const unsigned char*)"OpenAI-Safety-Identifier:",
                (const unsigned char*)safetyId, (int)strlen(safetyId), p, end))
            return -1;
        return 0;
    }
    case LWS_CALLBACK_CLIENT_ESTABLISHED:
        return self->onEstablished(wsi);
    case LWS_CALLBACK_CLIENT_RECEIVE:
        return self->onReceive(wsi, (const char*)in, len,
                               lws_is_final_fragment(wsi));
    case LWS_CALLBACK_CLIENT_WRITEABLE:
        return self->onWriteable(wsi);
    case LWS_CALLBACK_CLIENT_CONNECTION_ERROR:
        if (in && len > 0)
            AG_LOG(ERROR, "[OpenAI] Connection error: %.*s", (int)len, (const char*)in);
        else
            AG_LOG(ERROR, "[OpenAI] Connection error (no details)");
        self->onClosed();
        return 0;
    case LWS_CALLBACK_CLIENT_CLOSED:
        self->onClosed();
        return 0;
    case LWS_CALLBACK_EVENT_WAIT_CANCELLED:
        self->onCancelled();
        return 0;
    default:
        return lws_callback_http_dummy(wsi, reason, nullptr, in, len);
    }
}

static const struct lws_protocols kProtocols[] = {
    {"realtime", callback_openai, 0, 65536, 0, nullptr, 0},
    LWS_PROTOCOL_LIST_TERM
};

// ── OpenAIWsClient ────────────────────────────────────────────────────────────

OpenAIWsClient::OpenAIWsClient(std::string apiKey, std::string srcLang, std::string dstLang)
    : apiKey_(std::move(apiKey)), srcLang_(std::move(srcLang)), dstLang_(std::move(dstLang)) {}

OpenAIWsClient::~OpenAIWsClient() { stop(); }

bool OpenAIWsClient::start() {
    lws_set_log_level(LLL_ERR | LLL_WARN, nullptr);

    lws_context_creation_info info{};
    info.port      = CONTEXT_PORT_NO_LISTEN;
    info.protocols = kProtocols;
    info.options   = LWS_SERVER_OPTION_DO_SSL_GLOBAL_INIT;
    info.user      = this;

    ctx_ = lws_create_context(&info);
    if (!ctx_) {
        AG_LOG(ERROR, "[OpenAI] Failed to create lws context");
        return false;
    }

    running_ = true;
    loopThread_ = std::thread(&OpenAIWsClient::eventLoop, this);
    return true;
}

void OpenAIWsClient::stop() {
    running_ = false;
    if (ctx_) lws_cancel_service(ctx_);
    if (loopThread_.joinable()) loopThread_.join();
    if (ctx_) { lws_context_destroy(ctx_); ctx_ = nullptr; }
    connected_ = false;
    wsi_       = nullptr;
}

void OpenAIWsClient::eventLoop() {
    const char* host  = "api.openai.com";
    const int   port  = 443;
    const char* path  = "/v1/realtime/translations?model=gpt-realtime-translate";

    lws_client_connect_info ci{};
    ci.context        = ctx_;
    ci.address        = host;
    ci.port           = port;
    ci.path           = path;
    ci.host           = host;
    ci.origin         = host;
    ci.protocol       = kProtocols[0].name;
    ci.ssl_connection = LCCSCF_USE_SSL;

    wsi_ = lws_client_connect_via_info(&ci);
    if (!wsi_) {
        AG_LOG(ERROR, "[OpenAI] lws_client_connect_via_info failed");
        running_ = false;
        return;
    }

    while (running_) {
        lws_service(ctx_, 10);  // 10 ms poll timeout
    }
}

// Called from event-loop thread when connection is established.
int OpenAIWsClient::onEstablished(struct lws* wsi) {
    wsi_       = wsi;
    connected_ = true;
    AG_LOG(INFO, "[OpenAI] WebSocket connected");

    std::string json =
        std::string("{\"type\":\"session.update\",\"session\":{\"audio\":{"
        "\"output\":{\"language\":\"") +
        dstLang_ + "\"}}}}";

    writeQueue_.push(json);
    lws_callback_on_writable(wsi);
    return 0;
}

// Called from event-loop thread on receiving data.
int OpenAIWsClient::onReceive(struct lws* wsi, const char* data, size_t len, bool isFinal) {
    recvBuf_.insert(recvBuf_.end(), data, data + len);
    if (!isFinal) return 0;

    std::string msg(recvBuf_.begin(), recvBuf_.end());
    recvBuf_.clear();
    parseMessage(msg);
    return 0;
}

// Called from event-loop thread when socket is writable.
int OpenAIWsClient::onWriteable(struct lws* wsi) {
    // Drain any pending audio into a JSON message.
    {
        std::lock_guard<std::mutex> lk(sendMu_);
        if (!pendingPcm_.empty()) {
            auto b64 = base64Encode(
                (const uint8_t*)pendingPcm_.data(),
                pendingPcm_.size() * sizeof(int16_t));
            pendingPcm_.clear();
            writeQueue_.push(
                "{\"type\":\"session.input_audio_buffer.append\",\"audio\":\"" + b64 + "\"}");
        }
    }

    if (writeQueue_.empty()) return 0;

    const std::string& msg = writeQueue_.front();
    size_t msgLen = msg.size();
    std::vector<uint8_t> buf(LWS_PRE + msgLen);
    memcpy(buf.data() + LWS_PRE, msg.data(), msgLen);
    lws_write(wsi, buf.data() + LWS_PRE, msgLen, LWS_WRITE_TEXT);
    writeQueue_.pop();

    if (!writeQueue_.empty()) lws_callback_on_writable(wsi);
    return 0;
}

void OpenAIWsClient::onClosed() {
    AG_LOG(INFO, "[OpenAI] WebSocket closed");
    connected_ = false;
    wsi_       = nullptr;
    running_   = false;
}

// Called when lws_cancel_service wakes the loop (from sendAudio cross-thread).
void OpenAIWsClient::onCancelled() {
    if (wsi_ && connected_) lws_callback_on_writable(wsi_);
}

void OpenAIWsClient::sendAudio(const int16_t* samples, int count) {
    if (!connected_ || count <= 0) return;
    {
        std::lock_guard<std::mutex> lk(sendMu_);
        pendingPcm_.insert(pendingPcm_.end(), samples, samples + count);
    }
    if (ctx_) lws_cancel_service(ctx_);  // thread-safe wake-up
}

void OpenAIWsClient::parseMessage(const std::string& msg) {
    try {
        auto j    = nlohmann::json::parse(msg);
        auto type = j.value("type", std::string{});

        if (type == "session.output_audio.delta") {
            auto delta = j.value("delta", std::string{});
            if (!delta.empty() && pcmCb_) {
                auto pcm = base64DecodePcm(delta);
                if (!pcm.empty()) pcmCb_(pcm);
            }
        } else if (type == "session.output_transcript.delta") {
            auto text = j.value("delta", std::string{});
            if (!text.empty()) {
                if (textCb_) textCb_(text);
                AG_LOG(INFO, "[OpenAI] output transcript: %s", text.c_str());
            }
        } else if (type == "session.input_transcript.delta") {
            auto text = j.value("delta", std::string{});
            if (!text.empty()) {
                AG_LOG(INFO, "[OpenAI] input transcript: %s", text.c_str());
            }
        } else if (type == "error") {
            AG_LOG(ERROR, "[OpenAI] server error: %s", msg.c_str());
        } else if (type == "session.created" || type == "session.updated") {
            AG_LOG(INFO, "[OpenAI] %s: %s", type.c_str(), msg.c_str());
        } else if (type == "session.closed") {
            AG_LOG(INFO, "[OpenAI] session closed by server");
        } else {
            AG_LOG(INFO, "[OpenAI] event: %s", msg.c_str());
        }
    } catch (const std::exception& e) {
        AG_LOG(ERROR, "[OpenAI] JSON parse error: %s (len=%zu)", e.what(), msg.size());
    }
}
