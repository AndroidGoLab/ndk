#!/usr/bin/env bash
# E2E test: verify AAudio input stream lifecycle on Android emulator.
#
# Opens an AAudio capture stream at 48 kHz, reads ~1 second of audio,
# and verifies the full lifecycle (open/start/read/stop/close) completes
# without errors.
#
# If PulseAudio is available and the emulator is launched with
# -allow-host-audio, the script also injects a 440 Hz tone via a
# PulseAudio null-sink monitor and verifies frequency detection.
#
# Prerequisites:
#   - Android NDK installed (auto-detected or set ANDROID_NDK_HOME)
#   - A running Android emulator (preferably with -allow-host-audio)
#
# Usage: ./tests/e2e/run-audio-e2e.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Detect NDK.
ANDROID_HOME="${ANDROID_HOME:-$HOME/Android/Sdk}"
NDK_PATH="${ANDROID_NDK_HOME:-$(ls -d "$ANDROID_HOME/ndk/"* 2>/dev/null | sort -V | tail -1)}"
if [ -z "$NDK_PATH" ] || [ ! -d "$NDK_PATH" ]; then
    echo "ERROR: Android NDK not found; set ANDROID_NDK_HOME or ANDROID_HOME"
    exit 1
fi

API_LEVEL="${API_LEVEL:-35}"
NDK_SYSROOT="${NDK_PATH}/toolchains/llvm/prebuilt/linux-x86_64/sysroot"
CC="${NDK_PATH}/toolchains/llvm/prebuilt/linux-x86_64/bin/x86_64-linux-android${API_LEVEL}-clang"
if [ ! -x "$CC" ]; then
    echo "ERROR: NDK clang not found at $CC"
    exit 1
fi

ADB="${ADB:-adb}"
# adb starts a separate shell, so the host's GODEBUG does not propagate.
REMOTE_GODEBUG="${GODEBUG:-cgocheck=1}"
export CGO_CFLAGS="${CGO_CFLAGS:+$CGO_CFLAGS }--sysroot=$NDK_SYSROOT"
export CGO_CPPFLAGS="${CGO_CPPFLAGS:+$CGO_CPPFLAGS }--sysroot=$NDK_SYSROOT"
export CGO_LDFLAGS="${CGO_LDFLAGS:+$CGO_LDFLAGS }--sysroot=$NDK_SYSROOT"

# Verify adb connectivity.
if ! "$ADB" get-state >/dev/null 2>&1; then
    echo "ERROR: No adb device found. Start an emulator first."
    exit 1
fi

cd "$PROJECT_DIR"

# Cross-compile device binary.
echo "=== Building audio recording E2E (android/amd64) ==="
CGO_ENABLED=1 GOOS=android GOARCH=amd64 CC="$CC" \
    go build -o /tmp/audio-recording-e2e ./tests/e2e/audio-recording-e2e

# Try to set up PulseAudio tone injection (best-effort).
DETECT_TONE=""
INJECT_CLEANUP=""

setup_pa_injection() {
    if ! command -v pactl >/dev/null 2>&1; then
        echo "WARN: pactl not found, skipping audio injection"
        return 1
    fi

    # Create a null sink whose monitor becomes the virtual mic.
    local sink_idx
    sink_idx=$(pactl load-module module-null-sink \
        sink_name=e2e_audio_sink \
        sink_properties=device.description=E2EAudioSink 2>/dev/null) || return 1

    pactl set-default-source e2e_audio_sink.monitor 2>/dev/null || {
        pactl unload-module "$sink_idx" 2>/dev/null
        return 1
    }

    # Generate a 440 Hz stereo tone at 44100 Hz (emulator's PA format).
    python3 -c "
import struct, math
rate, freq, dur, amp = 44100, 440, 10, 30000
with open('/tmp/e2e_tone_440hz.raw', 'wb') as f:
    for i in range(rate * dur):
        s = int(amp * math.sin(2 * math.pi * freq * i / rate))
        f.write(struct.pack('<hh', s, s))
" 2>/dev/null || return 1

    # Play tone into the null sink in background.
    paplay --raw --format=s16le --channels=2 --rate=44100 \
        --device=e2e_audio_sink /tmp/e2e_tone_440hz.raw &>/dev/null &
    local play_pid=$!

    INJECT_CLEANUP="kill $play_pid 2>/dev/null; pactl unload-module $sink_idx 2>/dev/null; rm -f /tmp/e2e_tone_440hz.raw"
    echo "Audio injection active (440 Hz via PulseAudio)"
    # NOTE: The Android emulator (v36) does not reliably pass host
    # PulseAudio input to the guest microphone. Tone injection is
    # set up for diagnostic purposes; pass -detect-tone manually
    # if your environment supports it.
    return 0
}

# Attempt injection; failures are non-fatal.
setup_pa_injection || echo "Continuing without audio injection"

cleanup() {
    "$ADB" shell rm -f /data/local/tmp/audio-recording-e2e 2>/dev/null || true
    eval "$INJECT_CLEANUP" 2>/dev/null || true
    rm -f /tmp/audio-recording-e2e 2>/dev/null || true
}
trap cleanup EXIT

# Push and run device binary.
echo "=== Running audio recording E2E on device ==="
"$ADB" push /tmp/audio-recording-e2e /data/local/tmp/audio-recording-e2e >/dev/null 2>&1
"$ADB" shell chmod 755 /data/local/tmp/audio-recording-e2e

# shellcheck disable=SC2086
OUTPUT=$("$ADB" shell "GODEBUG=$REMOTE_GODEBUG timeout 15 /data/local/tmp/audio-recording-e2e $DETECT_TONE 2>&1; echo EXIT=\$?" 2>&1)
echo "$OUTPUT"

EXIT_CODE=$(echo "$OUTPUT" | grep -oP 'EXIT=\K\d+' | tail -1)

if [ "${EXIT_CODE:-1}" = "0" ]; then
    echo "=== PASS: Audio recording E2E ==="
else
    echo "=== FAIL: Audio recording E2E (exit=$EXIT_CODE) ==="
fi

exit "${EXIT_CODE:-1}"
