#pragma once
#include <atomic>
#include <functional>
#include <mutex>
#include <queue>
#include <string>
#include <thread>
#include <vector>

struct lws_context;
struct lws;

// WebSocket client for OpenAI Realtime Translation API.
// Streams PCM16 24 kHz mono audio to OpenAI and receives translated
// PCM16 24 kHz mono audio back via callback.
//
// Thread safety: sendAudio() may be called from any thread.
// All other methods must be called from the owning thread.
class OpenAIWsClient {
public:
    // Called with decoded 24 kHz PCM16 mono samples from OpenAI
    using PcmCallback = std::function<void(const std::vector<int16_t>&)>;
    // Called with transcript text. kind=0: input (source lang), kind=1: output (target lang).
    // isFinal=true on utterance boundary (.done/.completed event).
    using TranscriptCallback = std::function<void(int kind, const std::string& text, bool isFinal)>;

    OpenAIWsClient(std::string apiKey, std::string srcLang, std::string dstLang);
    ~OpenAIWsClient();

    void setPcmCallback(PcmCallback cb)             { pcmCb_        = std::move(cb); }
    void setTranscriptCallback(TranscriptCallback cb) { transcriptCb_ = std::move(cb); }

    const std::string& apiKey()  const { return apiKey_; }
    const std::string& srcLang() const { return srcLang_; }
    const std::string& dstLang() const { return dstLang_; }

    // Connect and start background event-loop thread.
    bool start();
    // Stop event loop and disconnect.
    void stop();

    bool isConnected() const { return connected_; }

    // Thread-safe. Enqueues 24 kHz PCM16 mono samples for sending.
    void sendAudio(const int16_t* samples, int count);

    // ── Internal — called from libwebsockets C callbacks ──────────────────
    int  onEstablished(struct lws* wsi);
    int  onReceive(struct lws* wsi, const char* data, size_t len, bool isFinal);
    int  onWriteable(struct lws* wsi);
    void onClosed();
    void onCancelled();

private:
    void eventLoop();
    void parseMessage(const std::string& msg);

    std::string apiKey_;
    std::string srcLang_;
    std::string dstLang_;

    PcmCallback        pcmCb_;
    TranscriptCallback transcriptCb_;

    lws_context* ctx_{nullptr};
    lws*         wsi_{nullptr};

    std::atomic<bool> connected_{false};
    std::atomic<bool> running_{false};
    std::thread       loopThread_;

    // Audio to send — protected by sendMu_
    std::mutex              sendMu_;
    std::vector<int16_t>    pendingPcm_;  // 24 kHz PCM16 mono samples

    // JSON messages to send — consumed in onWriteable (event-loop thread only)
    std::queue<std::string> writeQueue_;

    // Partial receive buffer for fragmented WebSocket messages
    std::vector<char> recvBuf_;
};
