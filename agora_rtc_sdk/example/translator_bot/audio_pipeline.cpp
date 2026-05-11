#include "audio_pipeline.h"

#include <algorithm>
#include <cstdio>
#include <cstring>
#include <samplerate.h>

// ── Resampler ───────────────────────────────────────────────────────────────

Resampler::Resampler(int srcRate, int dstRate, int channels)
    : ratio_((double)dstRate / srcRate), channels_(channels) {
    int err = 0;
    state_ = src_new(SRC_SINC_MEDIUM_QUALITY, channels, &err);
    if (!state_) {
        fprintf(stderr, "[Resampler] src_new failed: %s\n", src_strerror(err));
    }
}

Resampler::~Resampler() {
    if (state_) src_delete((SRC_STATE*)state_);
}

std::vector<int16_t> Resampler::process(const int16_t* in, int inFrames) {
    if (!state_ || inFrames <= 0) return {};

    std::vector<float> fIn(inFrames * channels_);
    src_short_to_float_array(in, fIn.data(), inFrames * channels_);

    int maxOut = (int)(inFrames * ratio_ * 1.1) + 8;
    std::vector<float> fOut(maxOut * channels_);

    SRC_DATA sd{};
    sd.data_in        = fIn.data();
    sd.data_out       = fOut.data();
    sd.input_frames   = inFrames;
    sd.output_frames  = maxOut;
    sd.src_ratio      = ratio_;
    sd.end_of_input   = 0;

    if (src_process((SRC_STATE*)state_, &sd) != 0) return {};

    std::vector<int16_t> out(sd.output_frames_gen * channels_);
    src_float_to_short_array(fOut.data(), out.data(), sd.output_frames_gen * channels_);
    return out;
}

// ── JitterBuffer ─────────────────────────────────────────────────────────────

JitterBuffer::JitterBuffer(int maxSamples) : maxSamples_(maxSamples) {}

void JitterBuffer::push(const int16_t* samples, int n) {
    std::lock_guard<std::mutex> lk(mu_);
    if ((int)buf_.size() + n > maxSamples_) {
        int drop = (int)buf_.size() + n - maxSamples_;
        buf_.erase(buf_.begin(), buf_.begin() + drop);
    }
    buf_.insert(buf_.end(), samples, samples + n);
}

void JitterBuffer::pop(int16_t* out, int n) {
    std::lock_guard<std::mutex> lk(mu_);
    int have = (int)buf_.size();
    int copy = std::min(have, n);
    for (int i = 0; i < copy; ++i) out[i] = buf_[i];
    buf_.erase(buf_.begin(), buf_.begin() + copy);
    if (copy < n) memset(out + copy, 0, (n - copy) * sizeof(int16_t));
}

int JitterBuffer::available() const {
    std::lock_guard<std::mutex> lk(mu_);
    return (int)buf_.size();
}
