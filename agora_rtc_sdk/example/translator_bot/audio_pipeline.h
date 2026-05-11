#pragma once
#include <cstdint>
#include <deque>
#include <mutex>
#include <vector>

// PCM16 resampler wrapping libsamplerate.
// Not thread-safe — each user should own one instance.
class Resampler {
public:
    Resampler(int srcRate, int dstRate, int channels = 1);
    ~Resampler();
    std::vector<int16_t> process(const int16_t* in, int inFrames);
private:
    void*  state_;
    double ratio_;
    int    channels_;
};

// Thread-safe PCM16 jitter buffer.
// Underrun fills silence rather than stalling.
class JitterBuffer {
public:
    explicit JitterBuffer(int maxSamples = 48000);  // ~3 s at 16 kHz
    void push(const int16_t* samples, int n);
    void pop(int16_t* out, int n);
    int  available() const;
private:
    mutable std::mutex    mu_;
    std::deque<int16_t>   buf_;
    int                   maxSamples_;
};
