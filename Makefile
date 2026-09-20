export PATH := $(HOME)/go/bin:$(PATH)

GODEBUG ?= cgocheck=1
export GODEBUG

ANDROID_HOME ?= $(HOME)/Android/Sdk
NDK_PATH ?= $(or $(ANDROID_NDK_HOME),$(shell ls -d $(ANDROID_HOME)/ndk/* 2>/dev/null | sort -V | tail -1))

# Full module list for NDK production builds.
MODULES  := aaudio camera sensor gles2 gles3 egl vulkan media \
            input nativewindow hardwarebuffer binder thermal \
            performancehint neuralnetworks trace logging font \
            imagedecoder midi multinetwork sync choreographer \
            configuration asset looper nativeactivity surfacecontrol \
            sharedmem permission bitmap storage surfacetexture \
            persistablebundle

# Modules with testdata fixtures (no NDK required).
FIXTURE_MODULES := $(notdir $(wildcard tools/pkg/specgen/testdata/*/))

NDK_SYSROOT := $(NDK_PATH)/toolchains/llvm/prebuilt/linux-x86_64/sysroot/usr/include
NDK_CGO_SYSROOT := $(NDK_PATH)/toolchains/llvm/prebuilt/linux-x86_64/sysroot
C2FFI_BIN   ?= c2ffi

API_LEVEL      ?= 35
NDK_CC_ARM64   := $(NDK_PATH)/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android$(API_LEVEL)-clang
NDK_CC_X86_64  := $(NDK_PATH)/toolchains/llvm/prebuilt/linux-x86_64/bin/x86_64-linux-android$(API_LEVEL)-clang
BUILD_DIR      := build

.PHONY: all capi specs idiomatic clean regen fixtures test lint check-examples check-no-capi \
        e2e e2e-build e2e-examples e2e-examples-test e2e-audio \
        ndkcli ndkcli-commands ndkcli-release install-ndk \
        proofs test-differential

all: specs capi idiomatic

# Stage 1+2: Generate specs from C headers via c2ffi (requires NDK + c2ffi)
specs:
	@for m in $(MODULES); do \
		manifest="capi/manifests/$$m.yaml"; \
		[ -f "$$manifest" ] || continue; \
		echo "specgen $$m (c2ffi)"; \
		go run ./tools/cmd/specgen \
			-manifest "$$manifest" \
			-ndk-include "$(NDK_SYSROOT)" \
			-c2ffi-bin "$(C2FFI_BIN)" \
			-out "spec/generated/$$m.yaml"; \
	done

# Stage 2: Generate raw CGo binding packages from specs + manifests
capi:
	@for m in $(MODULES); do \
		manifest="capi/manifests/$$m.yaml"; \
		spec="spec/generated/$$m.yaml"; \
		[ -f "$$manifest" ] || continue; \
		[ -f "$$spec" ] || continue; \
		overlay="spec/overlays/$$m.yaml"; \
		overlay_flag=""; \
		[ -f "$$overlay" ] && overlay_flag="-overlay $$overlay"; \
		echo "capigen $$m"; \
		go run ./tools/cmd/capigen \
			-spec "$$spec" \
			-manifest "$$manifest" \
			$$overlay_flag \
			-out "capi/$$m/"; \
	done

# Stage 2 (fixture mode): Extract specs from testdata fixtures
specs-fixtures:
	@for m in $(FIXTURE_MODULES); do \
		case $$m in simple|edgecases) continue;; esac; \
		echo "specgen $$m (fixture)"; \
		go run ./tools/cmd/specgen \
			-module $$m \
			-pkg tools/pkg/specgen/testdata/$$m \
			-out spec/generated/$$m.yaml; \
	done

# Stage 3: Generate idiomatic Go from specs + overlays
idiomatic:
	@for overlay in spec/overlays/*.yaml; do \
		m=$$(basename "$$overlay" .yaml); \
		spec="spec/generated/$$m.yaml"; \
		[ -f "$$spec" ] || continue; \
		goname=$$(grep 'go_name:' "$$overlay" | head -1 | awk '{print $$2}'); \
		[ -z "$$goname" ] && goname="$$m"; \
		echo "idiomgen $$m -> $$goname/"; \
		go run ./tools/cmd/idiomgen \
			-spec "$$spec" \
			-overlay "$$overlay" \
			-templates templates/ \
			-capi-dir "capi/$$m/" \
			-out "$$goname/"; \
	done

# Generate everything from fixtures (no NDK required)
fixtures: specs-fixtures idiomatic

# Regenerate everything from scratch (requires NDK + c2ffi)
regen: clean specs capi idiomatic

test:
	go test $$(go list ./... | grep -v -E '/(capi|cmd|tests|examples)/|/ndk/[a-z][a-z0-9]*$$') -count=1

# Build Lean 4 proofs (requires elan/lake toolchain).
proofs:
	cd proofs && lake build

# Run differential tests comparing Go implementations against Lean oracle.
# Requires: elan/lake toolchain. Builds the oracle automatically.
test-differential: proofs
	go test ./tools/pkg/capigen/ ./tools/pkg/idiomgen/ -run TestDifferential -v

# Verify cmd/ and examples/ do not import capi packages.
check-no-capi:
	@if grep -rl '"github.com/xaionaro-go/ndk/capi' cmd/ examples/ 2>/dev/null; then \
		echo "ERROR: cmd/ and examples/ must not import capi packages"; \
		exit 1; \
	fi
	@echo "OK: no capi imports in cmd/ or examples/"

lint:
	@which golangci-lint >/dev/null 2>&1 || { echo "Installing golangci-lint..."; go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest; }
	golangci-lint run ./tools/...

# Cross-compile all examples and ndkcli for Android arm64 to catch compile errors (requires NDK)
check-examples:
	CGO_ENABLED=1 GOOS=android GOARCH=arm64 CC=$(NDK_CC_ARM64) \
		CGO_CFLAGS="$(CGO_CFLAGS) --sysroot=$(NDK_CGO_SYSROOT)" CGO_CPPFLAGS="$(CGO_CPPFLAGS) --sysroot=$(NDK_CGO_SYSROOT)" CGO_LDFLAGS="$(CGO_LDFLAGS) --sysroot=$(NDK_CGO_SYSROOT)" \
		go build ./examples/... ./cmd/ndkcli/

# Cross-compile E2E test binary for Android x86_64 (requires NDK)
e2e-build:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 GOOS=android GOARCH=amd64 CC=$(NDK_CC_X86_64) \
		CGO_CFLAGS="$(CGO_CFLAGS) --sysroot=$(NDK_CGO_SYSROOT)" CGO_CPPFLAGS="$(CGO_CPPFLAGS) --sysroot=$(NDK_CGO_SYSROOT)" CGO_LDFLAGS="$(CGO_LDFLAGS) --sysroot=$(NDK_CGO_SYSROOT)" \
		go build -o $(BUILD_DIR)/e2e_test ./tests/e2e

# Run full E2E test on Android emulator (requires SDK + NDK + KVM)
e2e: e2e-build
	ANDROID_HOME=$(dir $(NDK_PATH)) ./tests/e2e/run.sh

# Run all examples on Android emulator (requires running emulator + NDK)
e2e-examples:
	./tests/e2e/run-examples.sh

# Run examples E2E Go test on Android emulator (requires running emulator + NDK)
e2e-examples-test:
	./tests/e2e/run-examples-test.sh

# Run audio recording E2E test (requires running emulator with audio + NDK)
e2e-audio:
	./tests/e2e/run-audio-e2e.sh

# Install Android NDK r28 (for CI; no-op if the correct version is present)
ANDROID_NDK_VERSION ?= r28
ANDROID_NDK_DIR_NAME ?= 28.0.13004108
NDK_INSTALL_PATH := $(ANDROID_HOME)/ndk/$(ANDROID_NDK_DIR_NAME)
install-ndk:
	@if [ -d "$(NDK_INSTALL_PATH)" ]; then echo "NDK $(ANDROID_NDK_VERSION) already at $(NDK_INSTALL_PATH)"; exit 0; fi; \
	echo "Installing Android NDK $(ANDROID_NDK_VERSION)..."; \
	wget -q "https://dl.google.com/android/repository/android-ndk-$(ANDROID_NDK_VERSION)-linux.zip" -O /tmp/ndk.zip; \
	unzip -q /tmp/ndk.zip -d /tmp/ndk-extract/; \
	mkdir -p "$(ANDROID_HOME)/ndk"; \
	mv /tmp/ndk-extract/android-ndk-* "$(NDK_INSTALL_PATH)"; \
	rm -f /tmp/ndk.zip; \
	echo "NDK installed to $(NDK_INSTALL_PATH)"

# Build ndkcli for Android arm64 (requires NDK)
ndkcli:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 GOOS=android GOARCH=arm64 CC=$(NDK_CC_ARM64) \
		CGO_CFLAGS="$(CGO_CFLAGS) --sysroot=$(NDK_CGO_SYSROOT)" CGO_CPPFLAGS="$(CGO_CPPFLAGS) --sysroot=$(NDK_CGO_SYSROOT)" CGO_LDFLAGS="$(CGO_LDFLAGS) --sysroot=$(NDK_CGO_SYSROOT)" \
		go build -o $(BUILD_DIR)/ndkcli ./cmd/ndkcli/

# Build release binaries for both architectures (stripped)
ndkcli-release:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 GOOS=android GOARCH=arm64 CC=$(NDK_CC_ARM64) \
		CGO_CFLAGS="$(CGO_CFLAGS) --sysroot=$(NDK_CGO_SYSROOT)" CGO_CPPFLAGS="$(CGO_CPPFLAGS) --sysroot=$(NDK_CGO_SYSROOT)" CGO_LDFLAGS="$(CGO_LDFLAGS) --sysroot=$(NDK_CGO_SYSROOT)" \
		go build -trimpath -ldflags="-s -w" -o $(BUILD_DIR)/ndkcli-android-arm64 ./cmd/ndkcli/
	CGO_ENABLED=1 GOOS=android GOARCH=amd64 CC=$(NDK_CC_X86_64) \
		CGO_CFLAGS="$(CGO_CFLAGS) --sysroot=$(NDK_CGO_SYSROOT)" CGO_CPPFLAGS="$(CGO_CPPFLAGS) --sysroot=$(NDK_CGO_SYSROOT)" CGO_LDFLAGS="$(CGO_LDFLAGS) --sysroot=$(NDK_CGO_SYSROOT)" \
		go build -trimpath -ldflags="-s -w" -o $(BUILD_DIR)/ndkcli-android-x86_64 ./cmd/ndkcli/

# Print all ndkcli subcommands (extracted from source, no binary needed)
ndkcli-commands:
	@go run ./tools/cmd/ndkcli-commands

clean:
	@for m in $(MODULES); do rm -rf "capi/$$m/"; done
	rm -rf spec/generated/

# --- APK Build (examples) ---

APK_API      ?= 35
APK_ARCH     ?= arm64

ifeq ($(APK_ARCH),arm64)
  APK_ABI        := arm64-v8a
  APK_GOARCH     := arm64
  APK_CC_PREFIX  := aarch64-linux-android$(APK_API)
else ifeq ($(APK_ARCH),x86_64)
  APK_ABI        := x86_64
  APK_GOARCH     := amd64
  APK_CC_PREFIX  := x86_64-linux-android$(APK_API)
endif

APK_CC         := $(NDK_PATH)/toolchains/llvm/prebuilt/linux-x86_64/bin/$(APK_CC_PREFIX)-clang
APK_BUILD_TOOLS := $(shell ls -d $(ANDROID_HOME)/build-tools/* 2>/dev/null | sort -V | tail -1)
APK_PLATFORM   := $(ANDROID_HOME)/platforms/android-$(APK_API)/android.jar
APK_KEYSTORE   := $(HOME)/.android/debug.keystore

$(APK_KEYSTORE):
	mkdir -p $(dir $@)
	keytool -genkeypair -v -keystore $@ -storepass android \
		-alias androiddebugkey -keypass android -keyalg RSA \
		-keysize 2048 -validity 10000 \
		-dname "CN=Debug,O=Debug,C=US"

.PHONY: apk-displaycamera

apk-displaycamera: $(APK_KEYSTORE)
	$(eval DIR := examples/camera/display)
	$(eval BUILD := $(DIR)/_build)
	@rm -rf $(BUILD)
	@mkdir -p $(BUILD)/lib/$(APK_ABI)
	CGO_ENABLED=1 GOOS=android GOARCH=$(APK_GOARCH) CC=$(APK_CC) \
		CGO_CFLAGS="$(CGO_CFLAGS) --sysroot=$(NDK_CGO_SYSROOT)" CGO_CPPFLAGS="$(CGO_CPPFLAGS) --sysroot=$(NDK_CGO_SYSROOT)" CGO_LDFLAGS="$(CGO_LDFLAGS) --sysroot=$(NDK_CGO_SYSROOT)" \
		go build -buildmode=c-shared \
		-o $(BUILD)/lib/$(APK_ABI)/libdisplaycamera.so \
		./$(DIR)
	@rm -f $(BUILD)/lib/$(APK_ABI)/libdisplaycamera.h
	$(APK_BUILD_TOOLS)/aapt package -f \
		-M $(DIR)/AndroidManifest.xml \
		-I $(APK_PLATFORM) \
		-F $(BUILD)/base.apk
	cd $(BUILD) && zip -r base.apk lib/
	$(APK_BUILD_TOOLS)/zipalign -f -p 4 \
		$(BUILD)/base.apk $(BUILD)/aligned.apk
	$(APK_BUILD_TOOLS)/apksigner sign \
		--ks $(APK_KEYSTORE) --ks-pass pass:android \
		--key-pass pass:android \
		$(BUILD)/aligned.apk
	mv $(BUILD)/aligned.apk $(DIR)/displaycamera.apk
	@rm -rf $(BUILD)
	@echo "APK: $(DIR)/displaycamera.apk"
