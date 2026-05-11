#include <atomic>
#include <chrono>
#include <csignal>
#include <cstring>
#include <memory>
#include <sstream>
#include <string>
#include <thread>

#include "AgoraMediaBase.h"
#include "AgoraRefCountedObject.h"
#include "IAgoraService.h"
#include "NGIAgoraAudioTrack.h"
#include "NGIAgoraLocalUser.h"
#include "NGIAgoraMediaNode.h"
#include "NGIAgoraMediaNodeFactory.h"
#include "NGIAgoraRtcConnection.h"
#include "common/log.h"
#include "common/opt_parser.h"
#include "common/sample_common.h"
#include "common/sample_connection_observer.h"
#include "common/sample_local_user_observer.h"

#include "audio_pipeline.h"
#include "openai_ws_client.h"

#define SAMPLE_RATE    16000
#define NUM_CHANNELS   1
#define OPENAI_RATE    24000
#define FRAME_MS       10
#define SAMPLES_PER_FRAME (SAMPLE_RATE * FRAME_MS / 1000)  // 160

// ── CLI options ───────────────────────────────────────────────────────────────

struct Options {
    std::string appId;
    std::string channelId;
    std::string speakerUid = "0";
    std::string botUid     = "2002";
    std::string srcLang    = "en";
    std::string dstLang    = "es";
};

// ── PCM observer: receives speaker audio and forwards to OpenAI ──────────────

class TranslatorPcmObserver : public agora::media::IAudioFrameObserverBase {
public:
    TranslatorPcmObserver(OpenAIWsClient* client, Resampler* up)
        : openai_(client), upsampler_(up) {}

    bool onPlaybackAudioFrameBeforeMixing(const char* /*channelId*/,
                                          agora::media::base::user_id_t uid,
                                          AudioFrame& frame) override {
        frameCount_++;
        if (frameCount_ % 500 == 1)
            AG_LOG(INFO, "[Audio] frames=%d uid=%s connected=%d samples=%d",
                   (int)frameCount_, uid, (int)openai_->isConnected(),
                   frame.samplesPerChannel);
        if (!openai_->isConnected()) return true;
        const int16_t* pcm = (const int16_t*)frame.buffer;
        auto up = upsampler_->process(pcm, frame.samplesPerChannel);
        if (!up.empty()) openai_->sendAudio(up.data(), (int)up.size());
        return true;
    }

    bool onPlaybackAudioFrame(const char*, AudioFrame&)          override { return true; }
    bool onRecordAudioFrame(const char*, AudioFrame&)            override { return true; }
    bool onMixedAudioFrame(const char*, AudioFrame&)             override { return true; }
    bool onEarMonitoringAudioFrame(AudioFrame&)                  override { return true; }
    AudioParams getEarMonitoringAudioParams()                    override { return {}; }
    int  getObservedAudioFramePosition()                         override {
        return agora::media::IAudioFrameObserverBase::AUDIO_FRAME_POSITION_BEFORE_MIXING;
    }
    AudioParams getPlaybackAudioParams()                         override { return {}; }
    AudioParams getRecordAudioParams()                           override { return {}; }
    AudioParams getMixedAudioParams()                            override { return {}; }

private:
    OpenAIWsClient* openai_;
    int frameCount_{0};
    Resampler*      upsampler_;
};

// ── Sender thread: drains jitter buffer → Agora PCM sender every 10 ms ───────

static void senderThread(agora::agora_refptr<agora::rtc::IAudioPcmDataSender> sender,
                         JitterBuffer* jb,
                         std::atomic<bool>* quit) {
    // Wait for 50 ms of audio before starting playback; stop when buffer empties.
    // Prevents within-utterance underruns from inter-packet gaps.
    static const int kFillThreshold = SAMPLES_PER_FRAME * 5;  // 50 ms

    int16_t frame[SAMPLES_PER_FRAME];
    auto next = std::chrono::steady_clock::now();
    bool playing = false;
    int underruns = 0;
    while (!quit->load()) {
        next += std::chrono::milliseconds(FRAME_MS);
        int avail = jb->available();
        if (!playing && avail >= kFillThreshold) playing = true;
        if (playing && avail == 0) { playing = false; underruns++; }

        if (playing) {
            jb->pop(frame, SAMPLES_PER_FRAME);
        } else {
            memset(frame, 0, sizeof(frame));
            if (underruns > 0 && avail == 0 && underruns % 50 == 0)
                AG_LOG(INFO, "[TX] underruns=%d", underruns);
        }
        sender->sendAudioPcmData(frame, 0, 0,
                                 SAMPLES_PER_FRAME,
                                 agora::rtc::TWO_BYTES_PER_SAMPLE,
                                 NUM_CHANNELS,
                                 SAMPLE_RATE);
        std::this_thread::sleep_until(next);
    }
}

// ── Signal handler ────────────────────────────────────────────────────────────

static std::atomic<bool> gQuit{false};
static void onSignal(int) { gQuit = true; }

// ── main ──────────────────────────────────────────────────────────────────────

int main(int argc, char* argv[]) {
    Options opts;
    opt_parser parser;
    parser.add_long_opt("token",      &opts.appId,      "Agora App ID (required)");
    parser.add_long_opt("channelId",  &opts.channelId,  "Channel name (required)");
    parser.add_long_opt("speakerUid", &opts.speakerUid, "UID of the speaker to translate");
    parser.add_long_opt("botUid",     &opts.botUid,     "UID the bot joins and publishes as");
    parser.add_long_opt("srcLang",    &opts.srcLang,    "Source language code (e.g. en)");
    parser.add_long_opt("dstLang",    &opts.dstLang,    "Target language code (e.g. es)");

    if (argc <= 1 || !parser.parse_opts(argc, argv)) {
        std::ostringstream ss;
        parser.print_usage(argv[0], ss);
        printf("%s\n", ss.str().c_str());
        printf("\nRequired env vars:\n  OPENAI_API_KEY\n");
        return -1;
    }

    const char* openaiKey = getenv("OPENAI_API_KEY");
    if (!openaiKey || !*openaiKey) {
        AG_LOG(ERROR, "OPENAI_API_KEY env var not set");
        return -1;
    }
    if (opts.appId.empty() || opts.channelId.empty()) {
        AG_LOG(ERROR, "Must provide --token (appId) and --channelId");
        return -1;
    }

    std::signal(SIGINT,  onSignal);
    std::signal(SIGTERM, onSignal);
    std::signal(SIGQUIT, onSignal);

    // ── Agora service (single global singleton) ───────────────────────────
    // audio device off, audio processor on, video off
    auto* svc = createAndInitAgoraService(false, true, false);
    if (!svc) { AG_LOG(ERROR, "Failed to init AgoraService"); return -1; }

    // ── Audio pipeline objects ────────────────────────────────────────────
    Resampler    upsampler(SAMPLE_RATE, OPENAI_RATE, NUM_CHANNELS);  // 16k→24k
    Resampler    downsampler(OPENAI_RATE, SAMPLE_RATE, NUM_CHANNELS); // 24k→16k
    JitterBuffer jbuf(SAMPLE_RATE * 3);  // 3 s buffer at 16 kHz

    // ── OpenAI client ─────────────────────────────────────────────────────
    OpenAIWsClient openai(openaiKey, opts.srcLang, opts.dstLang);
    openai.setPcmCallback([&](const std::vector<int16_t>& pcm24k) {
        auto pcm16k = downsampler.process(pcm24k.data(), (int)pcm24k.size());
        if (!pcm16k.empty()) jbuf.push(pcm16k.data(), (int)pcm16k.size());
    });

    if (!openai.start()) {
        AG_LOG(ERROR, "Failed to start OpenAI WS client");
        svc->release();
        return -1;
    }
    AG_LOG(INFO, "OpenAI WS client started; waiting for connection...");

    // ── Bot connection (broadcaster — subscribes to speaker + publishes translation) ──
    agora::rtc::RtcConnectionConfiguration botCfg;
    botCfg.channelProfile           = agora::CHANNEL_PROFILE_LIVE_BROADCASTING;
    botCfg.clientRoleType           = agora::rtc::CLIENT_ROLE_BROADCASTER;
    botCfg.autoSubscribeAudio       = false;
    botCfg.autoSubscribeVideo       = false;
    botCfg.enableAudioRecordingOrPlayout = false;

    auto botConn = svc->createRtcConnection(botCfg);
    if (!botConn) { AG_LOG(ERROR, "Failed to create bot connection"); return -1; }

    auto connObs = std::make_shared<SampleConnectionObserver>();
    botConn->registerObserver(connObs.get());

    auto userObs = std::make_shared<SampleLocalUserObserver>(botConn->getLocalUser());
    auto pcmObs  = std::make_shared<TranslatorPcmObserver>(&openai, &upsampler);

    if (botConn->getLocalUser()->setPlaybackAudioFrameBeforeMixingParameters(
            NUM_CHANNELS, SAMPLE_RATE) != 0) {
        AG_LOG(ERROR, "Failed to set audio frame parameters");
        return -1;
    }
    userObs->setAudioFrameObserver(pcmObs.get());

    auto factory = svc->createMediaNodeFactory();
    if (!factory) { AG_LOG(ERROR, "Failed to create media node factory"); return -1; }

    auto pcmSender = factory->createAudioPcmDataSender();
    if (!pcmSender) { AG_LOG(ERROR, "Failed to create PCM sender"); return -1; }

    auto audioTrack = svc->createCustomAudioTrack(pcmSender);
    if (!audioTrack) { AG_LOG(ERROR, "Failed to create audio track"); return -1; }

    audioTrack->setEnabled(true);
    botConn->getLocalUser()->publishAudio(audioTrack);

    if (opts.speakerUid == "0" || opts.speakerUid.empty()) {
        AG_LOG(INFO, "Subscribing to all remote audio");
        botConn->getLocalUser()->subscribeAllAudio();
    } else {
        AG_LOG(INFO, "Subscribing to speaker UID %s", opts.speakerUid.c_str());
        botConn->getLocalUser()->subscribeAudio(opts.speakerUid.c_str());
    }

    if (botConn->connect(opts.appId.c_str(), opts.channelId.c_str(),
                         opts.botUid.c_str())) {
        AG_LOG(ERROR, "Failed to connect bot to channel");
        return -1;
    }
    connObs->waitUntilConnected(5000);
    AG_LOG(INFO, "Bot connected as UID %s", opts.botUid.c_str());

    // ── Start sender thread ───────────────────────────────────────────────
    std::atomic<bool> senderQuit{false};
    std::thread sender(senderThread, pcmSender, &jbuf, &senderQuit);

    AG_LOG(INFO, "Translator bot running. Speaker=%s  Bot=%s  %s→%s",
           opts.speakerUid.c_str(), opts.botUid.c_str(),
           opts.srcLang.c_str(), opts.dstLang.c_str());
    AG_LOG(INFO, "Listeners should subscribe to UID %s", opts.botUid.c_str());
    AG_LOG(INFO, "Press Ctrl+C to stop.");

    while (!gQuit) { std::this_thread::sleep_for(std::chrono::milliseconds(100)); }

    AG_LOG(INFO, "Shutting down...");

    // ── Cleanup ───────────────────────────────────────────────────────────
    senderQuit = true;
    sender.join();

    openai.stop();

    userObs->unsetAudioFrameObserver();

    botConn->getLocalUser()->unpublishAudio(audioTrack);
    botConn->getLocalUser()->unsubscribeAllAudio();
    botConn->unregisterObserver(connObs.get());
    botConn->disconnect();

    // Release in reverse creation order
    userObs.reset();
    pcmObs.reset();
    connObs.reset();
    pcmSender  = nullptr;
    audioTrack = nullptr;
    factory    = nullptr;
    botConn    = nullptr;
    svc->release();
    svc = nullptr;

    AG_LOG(INFO, "Done.");
    return 0;
}
