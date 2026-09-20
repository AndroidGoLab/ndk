#!/usr/bin/env bash
# E2E test: cross-compile all examples and run each on Android emulator.
#
# Expects a running emulator (adb device available).
# Cross-compiles each example for android/amd64, pushes to /data/local/tmp,
# runs with a timeout, and reports pass/fail.
#
# Prerequisites:
#   - Android NDK installed (auto-detected or set ANDROID_NDK_HOME)
#   - A running Android emulator or device (adb accessible)
#
# Usage: ./tests/e2e/run-examples.sh

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
TIMEOUT="${TIMEOUT:-15}"
# adb starts a separate shell, so the host's GODEBUG does not propagate.
REMOTE_GODEBUG="${GODEBUG:-cgocheck=1}"

# Verify adb connectivity.
if ! "$ADB" get-state >/dev/null 2>&1; then
    echo "ERROR: No adb device found. Start an emulator first."
    exit 1
fi

cd "$PROJECT_DIR"

PASS=0
FAIL=0
FAILURES=()

for main_go in $(find examples/ -name main.go -not -path "*/_build/*" | sort); do
    pkg=$(dirname "$main_go")
    name=$(echo "$pkg" | tr '/' '_')
    bin="/tmp/e2e_$name"

    # Build.
    if ! CGO_ENABLED=1 GOOS=android GOARCH=amd64 CC="$CC" go build -o "$bin" "./$pkg" 2>/tmp/build_err.txt; then
        echo "FAIL (build) $pkg"
        FAIL=$((FAIL + 1))
        FAILURES+=("$pkg (build)")
        continue
    fi

    # Push and run.
    "$ADB" push "$bin" "/data/local/tmp/e2e_$name" >/dev/null 2>&1
    "$ADB" shell "chmod 755 /data/local/tmp/e2e_$name"

    output=$("$ADB" shell "GODEBUG=$REMOTE_GODEBUG timeout $TIMEOUT /data/local/tmp/e2e_$name 2>&1; echo EXIT=\$?" 2>&1)
    exit_code=$(echo "$output" | grep -oP 'EXIT=\K\d+' | tail -1)

    if [ "$exit_code" = "0" ]; then
        echo "PASS $pkg"
        PASS=$((PASS + 1))
    else
        echo "FAIL (exit=$exit_code) $pkg"
        FAIL=$((FAIL + 1))
        FAILURES+=("$pkg (exit=$exit_code)")
    fi

    # Cleanup.
    "$ADB" shell "rm -f /data/local/tmp/e2e_$name" 2>/dev/null || true
    rm -f "$bin"
done

echo ""
echo "========== RESULTS =========="
echo "PASS: $PASS"
echo "FAIL: $FAIL"
echo "TOTAL: $((PASS + FAIL))"

if [ ${#FAILURES[@]} -gt 0 ]; then
    echo ""
    echo "Failures:"
    for f in "${FAILURES[@]}"; do
        echo "  $f"
    done
    exit 1
fi
