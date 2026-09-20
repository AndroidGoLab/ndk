#!/usr/bin/env bash
# E2E: cross-compile the Go test binary with the "e2e" build tag,
# push it to an Android emulator, and run it via adb.
#
# Prerequisites:
#   - Android NDK installed (auto-detected or set ANDROID_NDK_HOME)
#   - A running Android emulator or device (adb accessible)
#
# Usage: ./tests/e2e/run-examples-test.sh

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
TOOLCHAIN_DIR="${NDK_PATH}/toolchains/llvm/prebuilt/linux-x86_64"
NDK_SYSROOT="${TOOLCHAIN_DIR}/sysroot"
CC="${TOOLCHAIN_DIR}/bin/x86_64-linux-android${API_LEVEL}-clang"
if [ ! -x "$CC" ]; then
    echo "ERROR: NDK clang not found at $CC"
    exit 1
fi

export CGO_CFLAGS="${CGO_CFLAGS:+$CGO_CFLAGS }--sysroot=$NDK_SYSROOT"
export CGO_CPPFLAGS="${CGO_CPPFLAGS:+$CGO_CPPFLAGS }--sysroot=$NDK_SYSROOT"
export CGO_LDFLAGS="${CGO_LDFLAGS:+$CGO_LDFLAGS }--sysroot=$NDK_SYSROOT"

ADB="${ADB:-adb}"
TIMEOUT="${TIMEOUT:-60}"
# adb starts a separate shell, so the host's GODEBUG does not propagate.
REMOTE_GODEBUG="${GODEBUG:-cgocheck=1}"

# Verify adb connectivity.
if ! "$ADB" get-state >/dev/null 2>&1; then
    echo "ERROR: No adb device found. Start an emulator first."
    exit 1
fi

cd "$PROJECT_DIR"

BIN="/tmp/examples_e2e.test"
NDKCLI_BIN="/tmp/ndkcli"

echo "=== Cross-compiling examples E2E test binary (android/amd64) ==="
CGO_ENABLED=1 GOOS=android GOARCH=amd64 CC="$CC" \
    go test -c -tags e2e -o "$BIN" ./tests/e2e/

echo "=== Cross-compiling ndkcli (android/amd64) ==="
CGO_ENABLED=1 GOOS=android GOARCH=amd64 CC="$CC" \
    go build -o "$NDKCLI_BIN" ./cmd/ndkcli/

echo "=== Pushing to device ==="
"$ADB" push "$BIN" /data/local/tmp/examples_e2e.test >/dev/null 2>&1
"$ADB" push "$NDKCLI_BIN" /data/local/tmp/ndkcli >/dev/null 2>&1
"$ADB" shell chmod 755 /data/local/tmp/examples_e2e.test
"$ADB" shell chmod 755 /data/local/tmp/ndkcli

echo "=== Running tests ==="
OUTPUT=$("$ADB" shell "GODEBUG=$REMOTE_GODEBUG timeout $TIMEOUT /data/local/tmp/examples_e2e.test -test.v 2>&1; echo EXIT=\$?" 2>&1)
echo "$OUTPUT"

EXIT_CODE=$(echo "$OUTPUT" | grep -oP 'EXIT=\K\d+' | tail -1)

# Cleanup.
"$ADB" shell rm -f /data/local/tmp/examples_e2e.test /data/local/tmp/ndkcli 2>/dev/null || true
rm -f "$BIN" "$NDKCLI_BIN" 2>/dev/null || true

if [ "${EXIT_CODE:-1}" = "0" ]; then
    echo "=== PASS: examples E2E tests ==="
else
    echo "=== FAIL: examples E2E tests (exit=$EXIT_CODE) ==="
fi

exit "${EXIT_CODE:-1}"
