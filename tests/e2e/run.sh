#!/usr/bin/env bash
# E2E test: cross-compile Go+CGo+NDK binary and run on Android emulator.
#
# Prerequisites:
#   - Android SDK with emulator and system image installed
#   - Android NDK r27+ installed
#   - KVM available (/dev/kvm)
#   - ANDROID_HOME set (e.g., ~/android-sdk)
#
# Usage: ./tests/e2e/run.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

: "${ANDROID_HOME:?ANDROID_HOME must be set}"
NDK_VERSION="${NDK_VERSION:-28.0.13004108}"
API_LEVEL="${API_LEVEL:-35}"
AVD_NAME="${AVD_NAME:-ndk_e2e}"
SYSTEM_IMAGE="system-images;android-${API_LEVEL};google_apis;x86_64"

NDK="${ANDROID_HOME}/ndk/${NDK_VERSION}"
NDK_SYSROOT="${NDK}/toolchains/llvm/prebuilt/linux-x86_64/sysroot"
CC="${NDK}/toolchains/llvm/prebuilt/linux-x86_64/bin/x86_64-linux-android${API_LEVEL}-clang"
ADB="${ANDROID_HOME}/platform-tools/adb"
EMULATOR="${ANDROID_HOME}/emulator/emulator"
AVDMANAGER="${ANDROID_HOME}/cmdline-tools/latest/bin/avdmanager"
# adb starts a separate shell, so the host's GODEBUG does not propagate.
REMOTE_GODEBUG="${GODEBUG:-cgocheck=1}"
export CGO_CFLAGS="${CGO_CFLAGS:+$CGO_CFLAGS }--sysroot=$NDK_SYSROOT"
export CGO_CPPFLAGS="${CGO_CPPFLAGS:+$CGO_CPPFLAGS }--sysroot=$NDK_SYSROOT"
export CGO_LDFLAGS="${CGO_LDFLAGS:+$CGO_LDFLAGS }--sysroot=$NDK_SYSROOT"

# Verify prerequisites
for tool in "$CC" "$ADB" "$EMULATOR"; do
    [ -x "$tool" ] || { echo "ERROR: $tool not found"; exit 1; }
done
[ -c /dev/kvm ] || { echo "ERROR: /dev/kvm not available"; exit 1; }

echo "=== Step 1: Cross-compile ==="
cd "$PROJECT_DIR"
CGO_ENABLED=1 GOOS=android GOARCH=amd64 CC="$CC" \
    go build -o tests/e2e/e2e_test ./tests/e2e
file tests/e2e/e2e_test

echo "=== Step 2: Create AVD (if needed) ==="
if [ ! -d "$HOME/.android/avd/${AVD_NAME}.avd" ]; then
    echo "no" | bash "$AVDMANAGER" create avd \
        -n "$AVD_NAME" -k "$SYSTEM_IMAGE" --force
fi

echo "=== Step 3: Start emulator ==="
ANDROID_SDK_ROOT="$ANDROID_HOME" "$EMULATOR" \
    -avd "$AVD_NAME" -no-window -no-boot-anim \
    -gpu swiftshader_indirect -no-metrics 2>/tmp/emulator_e2e.log &
EMULATOR_PID=$!
trap "kill $EMULATOR_PID 2>/dev/null || true" EXIT

echo "Waiting for boot..."
for i in $(seq 1 60); do
    RESULT=$("$ADB" shell getprop sys.boot_completed 2>/dev/null | tr -d '\r\n')
    if [ "$RESULT" = "1" ]; then
        echo "Emulator booted after ~$((i*5))s"
        break
    fi
    if ! kill -0 "$EMULATOR_PID" 2>/dev/null; then
        echo "ERROR: Emulator process died"
        tail -20 /tmp/emulator_e2e.log
        exit 1
    fi
    sleep 5
done

echo "=== Step 4: Run E2E test ==="
"$ADB" push tests/e2e/e2e_test /data/local/tmp/
"$ADB" shell chmod 755 /data/local/tmp/e2e_test
"$ADB" shell "GODEBUG=$REMOTE_GODEBUG /data/local/tmp/e2e_test"
EXIT_CODE=$?

echo "=== Step 5: Verify logcat ==="
"$ADB" logcat -d -s "ndk-e2e"

exit $EXIT_CODE
