package capigen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndroidGoLab/ndk/tools/pkg/specmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func looperSpec() *specmodel.Spec {
	return &specmodel.Spec{
		Module:        "looper",
		SourcePackage: "github.com/AndroidGoLab/ndk/capi/looper",
		Types: map[string]specmodel.TypeDef{
			"ALooper": {
				Kind:   "opaque_ptr",
				CType:  "ALooper",
				GoType: "*C.ALooper",
			},
		},
		Enums: map[string][]specmodel.EnumValue{
			"ALOOPER_POLL": {
				{Name: "ALOOPER_POLL_WAKE", Value: -1},
				{Name: "ALOOPER_POLL_CALLBACK", Value: -2},
				{Name: "ALOOPER_POLL_TIMEOUT", Value: -3},
				{Name: "ALOOPER_POLL_ERROR", Value: -4},
			},
			"ALOOPER_EVENT": {
				{Name: "ALOOPER_EVENT_INPUT", Value: 1},
				{Name: "ALOOPER_EVENT_OUTPUT", Value: 2},
				{Name: "ALOOPER_EVENT_ERROR", Value: 4},
			},
		},
		Functions: map[string]specmodel.FuncDef{
			"ALooper_forThread": {
				CName:   "ALooper_forThread",
				Returns: "*ALooper",
			},
			"ALooper_addFd": {
				CName: "ALooper_addFd",
				Params: []specmodel.Param{
					{Name: "looper", Type: "*ALooper"},
					{Name: "fd", Type: "int32"},
					{Name: "ident", Type: "int32"},
					{Name: "events", Type: "int32"},
					{Name: "callback", Type: "ALooper_callbackFunc"},
					{Name: "data", Type: "unsafe.Pointer"},
				},
				Returns: "int32",
			},
			"ALooper_acquire": {
				CName: "ALooper_acquire",
				Params: []specmodel.Param{
					{Name: "looper", Type: "*ALooper"},
				},
			},
		},
		Callbacks: map[string]specmodel.CallbackDef{
			"ALooper_callbackFunc": {
				Params: []specmodel.Param{
					{Name: "fd", Type: "int32"},
					{Name: "events", Type: "int32"},
					{Name: "data", Type: "unsafe.Pointer"},
				},
				Returns: "int32",
			},
		},
	}
}

func looperManifest() *Manifest {
	m := &Manifest{}
	m.Generator.PackageName = "looper"
	m.Generator.PackageDescription = "Raw CGo bindings for Android looper"
	m.Generator.Includes = []string{"android/looper.h"}
	m.Generator.FlagGroups = []FlagGroup{
		{Name: "LDFLAGS", Flags: []string{"-landroid"}},
	}
	return m
}

func nativeWindowFrameRateSpec() *specmodel.Spec {
	return &specmodel.Spec{
		Module:        "nativewindow",
		SourcePackage: "github.com/AndroidGoLab/ndk/capi/nativewindow",
		Types: map[string]specmodel.TypeDef{
			"ANativeWindow": {
				Kind:   "opaque_ptr",
				CType:  "ANativeWindow",
				GoType: "*C.ANativeWindow",
			},
		},
		Enums: map[string][]specmodel.EnumValue{
			"ANativeWindow_ChangeFrameRateStrategy": {
				{Name: "ANATIVEWINDOW_CHANGE_FRAME_RATE_ONLY_IF_SEAMLESS", Value: 0},
			},
			"ANativeWindow_FrameRateCompatibility": {
				{Name: "ANATIVEWINDOW_FRAME_RATE_COMPATIBILITY_DEFAULT", Value: 0},
			},
		},
		Functions: map[string]specmodel.FuncDef{
			"ANativeWindow_clearFrameRate": {
				CName: "ANativeWindow_clearFrameRate",
				Params: []specmodel.Param{
					{Name: "window", Type: "*ANativeWindow"},
				},
				Returns: "int32",
			},
			"ANativeWindow_setFrameRateWithChangeStrategy": {
				CName: "ANativeWindow_setFrameRateWithChangeStrategy",
				Params: []specmodel.Param{
					{Name: "window", Type: "*ANativeWindow"},
					{Name: "frameRate", Type: "float32"},
					{Name: "compatibility", Type: "int8"},
					{Name: "changeFrameRateStrategy", Type: "int8"},
				},
				Returns: "int32",
			},
		},
	}
}

func nativeWindowManifest() *Manifest {
	m := &Manifest{}
	m.Generator.PackageName = "nativewindow"
	m.Generator.PackageDescription = "Raw CGo bindings for Android native window"
	m.Generator.Includes = []string{"android/native_window.h"}
	m.Generator.FlagGroups = []FlagGroup{
		{Name: "LDFLAGS", Flags: []string{"-landroid", "-lnativewindow"}},
	}
	m.Generator.FunctionAliases = []FunctionAlias{
		{
			Name:   "ANativeWindow_clearFrameRate",
			Target: "ANativeWindow_setFrameRateWithChangeStrategy",
			Args: []FunctionAliasArg{
				{Param: "window"},
				{Literal: "0"},
				{Literal: "ANATIVEWINDOW_FRAME_RATE_COMPATIBILITY_DEFAULT"},
				{Literal: "ANATIVEWINDOW_CHANGE_FRAME_RATE_ONLY_IF_SEAMLESS"},
			},
		},
	}
	return m
}

func TestGeneratePackage(t *testing.T) {
	spec := looperSpec()
	manifest := looperManifest()

	outDir := t.TempDir()
	err := GeneratePackage(spec, manifest, outDir)
	require.NoError(t, err)

	// Verify all expected files are created.
	expectedFiles := []string{
		"doc.go",
		"types.go",
		"const.go",
		"cgo_helpers.go",
		"cgo_helpers.h",
		"looper.go",
	}
	for _, f := range expectedFiles {
		path := filepath.Join(outDir, f)
		_, err := os.Stat(path)
		assert.NoError(t, err, "missing file: %s", f)
	}
}

func TestGeneratePackageWithAPILevels(t *testing.T) {
	spec := looperSpec()
	// Add an API-36 function to the spec.
	spec.Functions["ALooper_newFeature"] = specmodel.FuncDef{
		CName: "ALooper_newFeature",
		Params: []specmodel.Param{
			{Name: "looper", Type: "*ALooper"},
		},
		Returns: "int32",
	}
	manifest := looperManifest()
	apiLevels := map[string]int{
		"ALooper_newFeature": 36,
	}

	outDir := t.TempDir()
	err := GeneratePackage(spec, manifest, outDir, apiLevels)
	require.NoError(t, err)

	// Base file should exist and not contain the API-36 function.
	basePath := filepath.Join(outDir, "looper.go")
	baseContent, err := os.ReadFile(basePath)
	require.NoError(t, err)
	assert.NotContains(t, string(baseContent), "ALooper_newFeature",
		"base file must not contain API-36 function")
	assert.Contains(t, string(baseContent), "ALooper_forThread",
		"base file must contain base-level functions")

	// API-36 file should exist with build tag and the function.
	api36Path := filepath.Join(outDir, "looper_api36.go")
	api36Content, err := os.ReadFile(api36Path)
	require.NoError(t, err)
	api36Str := string(api36Content)
	assert.Contains(t, api36Str, "//go:build android_ndk36",
		"API-36 file must have build tag")
	assert.Contains(t, api36Str, "ALooper_newFeature",
		"API-36 file must contain the higher-API function")
	assert.Contains(t, api36Str, "#undef __ANDROID_MIN_SDK_VERSION__",
		"API-36 preamble must undef the SDK version")
	assert.Contains(t, api36Str, "#define __ANDROID_MIN_SDK_VERSION__ 36",
		"API-36 preamble must define SDK version 36")
	assert.Contains(t, api36Str, "__ANDROID_UNAVAILABLE_SYMBOLS_ARE_WEAK__",
		"API-36 preamble must enable weak symbols")
}

func TestGeneratePackageWithoutAPILevels(t *testing.T) {
	spec := looperSpec()
	manifest := looperManifest()

	outDir := t.TempDir()
	// No API levels — should produce the same output as before.
	err := GeneratePackage(spec, manifest, outDir)
	require.NoError(t, err)

	basePath := filepath.Join(outDir, "looper.go")
	baseContent, err := os.ReadFile(basePath)
	require.NoError(t, err)
	assert.Contains(t, string(baseContent), "ALooper_forThread")
	assert.Contains(t, string(baseContent), "ALooper_addFd")

	// No API-level files should be generated.
	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), "_api",
			"no API-level files should exist without API levels")
	}
}

func TestBuildAPILevelPreamble(t *testing.T) {
	manifest := looperManifest()
	preamble := buildAPILevelPreamble(manifest, 36)

	assert.Contains(t, preamble, "#undef __ANDROID_MIN_SDK_VERSION__")
	assert.Contains(t, preamble, "#define __ANDROID_MIN_SDK_VERSION__ 36")
	assert.Contains(t, preamble, "#define __ANDROID_UNAVAILABLE_SYMBOLS_ARE_WEAK__ 1")
	assert.Contains(t, preamble, "#cgo LDFLAGS: -landroid")
	assert.Contains(t, preamble, "#include \"android/looper.h\"")
}

func TestGenerateDocGo(t *testing.T) {
	content := generateDocGo("looper", "Raw CGo bindings for Android looper")
	assert.Contains(t, content, "package looper")
	assert.Contains(t, content, "DO NOT EDIT")
	assert.Contains(t, content, "Raw CGo bindings for Android looper")
}

func TestGenerateTypesGo(t *testing.T) {
	spec := looperSpec()
	preamble := buildCGoPreamble(looperManifest())
	callbackSet := map[string]bool{"ALooper_callbackFunc": true}

	content := generateTypesGo("looper", preamble, spec, callbackSet, nil, nil)

	assert.Contains(t, content, "type ALooper C.ALooper")
	assert.Contains(t, content, "type ALooper_callbackFunc func(")
	assert.Contains(t, content, "fd int32")
	assert.Contains(t, content, "data unsafe.Pointer")
	assert.Contains(t, content, ") int32")
}

func TestGenerateConstGo(t *testing.T) {
	spec := looperSpec()
	preamble := buildCGoPreamble(looperManifest())
	enumTypedefSet := buildEnumTypedefSet(spec)

	content := generateConstGo("looper", preamble, spec, enumTypedefSet)

	assert.Contains(t, content, "ALOOPER_POLL_WAKE = -1")
	assert.Contains(t, content, "ALOOPER_POLL_CALLBACK = -2")
	assert.Contains(t, content, "ALOOPER_EVENT_INPUT = 1")
}

func TestGenerateConstGoTypedEnum(t *testing.T) {
	spec := &specmodel.Spec{
		Types: map[string]specmodel.TypeDef{
			"camera_status_t": {
				Kind:   "typedef_int32",
				CType:  "camera_status_t",
				GoType: "int32",
			},
		},
		Enums: map[string][]specmodel.EnumValue{
			"camera_status_t": {
				{Name: "ACAMERA_OK", Value: 0},
				{Name: "ACAMERA_ERROR_UNKNOWN", Value: -10000},
			},
		},
	}
	enumTypedefSet := buildEnumTypedefSet(spec)
	content := generateConstGo("camera", "", spec, enumTypedefSet)

	assert.Contains(t, content, "ACAMERA_OK Camera_status_t = 0")
	assert.Contains(t, content, "ACAMERA_ERROR_UNKNOWN Camera_status_t = -10000")
}

func TestGenerateCgoHelpersH(t *testing.T) {
	spec := looperSpec()
	manifest := looperManifest()

	content := generateCgoHelpersH("looper", manifest, spec, nil)

	assert.Contains(t, content, "#include \"android/looper.h\"")
	assert.Contains(t, content, "#pragma once")
	assert.Contains(t, content, "#define __CGOGEN 1")
	// Callback proxy declaration.
	assert.Contains(t, content, "ALooper_callbackFunc_")
	assert.Contains(t, content, "int fd")
	assert.Contains(t, content, "void* data")
}

func TestGenerateFunctionsGo(t *testing.T) {
	spec := looperSpec()
	callbackSet := map[string]bool{"ALooper_callbackFunc": true}
	preamble := buildCGoPreamble(looperManifest())

	content := generateFunctionsGo("looper", preamble, spec, callbackSet, nil, nil, nil)

	// ALooper_forThread returns *ALooper.
	assert.Contains(t, content, "func ALooper_forThread() *ALooper")
	assert.Contains(t, content, "C.ALooper_forThread()")
	assert.Contains(t, content, "(*ALooper)(unsafe.Pointer(__ret))")
	assert.NotContains(t, content, "unsafe.Pointer(&__ret)", "pointer returns must not take address of __ret")

	// ALooper_addFd has parameters.
	assert.Contains(t, content, "func ALooper_addFd(")
	assert.Contains(t, content, "looper *ALooper")
	assert.Contains(t, content, "fd int32")
	assert.Contains(t, content, "callback ALooper_callbackFunc")
	assert.Contains(t, content, "data unsafe.Pointer")
	assert.Contains(t, content, "C.ALooper_addFd(")
	assert.Contains(t, content, "(*C.ALooper)(unsafe.Pointer(looper))")
	assert.Contains(t, content, ".PassValue()")
	assert.Contains(t, content, "runtime.KeepAlive(")

	// ALooper_acquire has no return.
	assert.Contains(t, content, "func ALooper_acquire(")
}

func TestGenerateFunctionAlias(t *testing.T) {
	spec := nativeWindowFrameRateSpec()
	manifest := nativeWindowManifest()

	outDir := t.TempDir()
	err := GeneratePackage(spec, manifest, outDir)
	require.NoError(t, err)

	basePath := filepath.Join(outDir, "nativewindow.go")
	baseContent, err := os.ReadFile(basePath)
	require.NoError(t, err)

	content := string(baseContent)
	assert.Contains(t, content, "func ANativeWindow_clearFrameRate(window *ANativeWindow) int32")
	assert.Contains(t, content, "return ANativeWindow_setFrameRateWithChangeStrategy(window, 0, ANATIVEWINDOW_FRAME_RATE_COMPATIBILITY_DEFAULT, ANATIVEWINDOW_CHANGE_FRAME_RATE_ONLY_IF_SEAMLESS)")
	assert.NotContains(t, content, "C.ANativeWindow_clearFrameRate")
}

func TestGeneratedNativeWindowDoesNotExposeClearFrameRateAlias(t *testing.T) {
	generatedFiles := []string{
		filepath.Join("..", "..", "..", "capi", "nativewindow", "nativewindow.go"),
		filepath.Join("..", "..", "..", "capi", "egl", "egl.go"),
		filepath.Join("..", "..", "..", "window", "window.go"),
		filepath.Join("..", "..", "..", "egl", "anative_window.go"),
	}

	for _, path := range generatedFiles {
		content, err := os.ReadFile(path)
		require.NoError(t, err)

		assert.NotContains(t, string(content), "ClearFrameRate", path)
		assert.NotContains(t, string(content), "clearFrameRate", path)
		assert.NotContains(t, string(content), "ANativeWindow_clearFrameRate", path)
	}
}

func TestCallbackHashSuffix(t *testing.T) {
	hash := callbackHashSuffix("looper", "ALooper_callbackFunc")
	// Hash is md5-based, not CRC32 like c-for-go. Both sides (caller and
	// proxy) are generated by capigen so consistency is all that matters.
	assert.Len(t, hash, 8)

	// Same callback name in different packages must produce different hashes.
	hash2 := callbackHashSuffix("otherpackage", "ALooper_callbackFunc")
	assert.NotEqual(t, hash, hash2, "different packages must produce different hashes")
}

func TestBuildCGoPreamble(t *testing.T) {
	manifest := looperManifest()
	preamble := buildCGoPreamble(manifest)

	assert.Contains(t, preamble, "#cgo LDFLAGS: -landroid")
	assert.Contains(t, preamble, "#include \"android/looper.h\"")
	assert.Contains(t, preamble, "#include <stdlib.h>")
	assert.Contains(t, preamble, "#include \"cgo_helpers.h\"")
}

func TestCallbackProxy(t *testing.T) {
	spec := looperSpec()
	cb := spec.Callbacks["ALooper_callbackFunc"]

	proxy := generateCallbackProxy("looper", "ALooper_callbackFunc", cb, spec, nil)

	hash := callbackHashSuffix("looper", "ALooper_callbackFunc")
	upperHash := strings.ToUpper(hash)

	assert.Contains(t, proxy, "PassRef()")
	assert.Contains(t, proxy, "PassValue()")
	assert.Contains(t, proxy, "//export ALooper_callbackFunc"+upperHash)
	assert.Contains(t, proxy, "func ALooper_callbackFunc"+upperHash+"(")
	assert.Contains(t, proxy, "aLooper_callbackFunc"+upperHash+"Func")

	// Verify it has parameter conversion.
	lines := strings.Split(proxy, "\n")
	hasExport := false
	for _, l := range lines {
		if strings.Contains(l, "//export") {
			hasExport = true
		}
	}
	assert.True(t, hasExport)
}

func TestParamConversionOpaque(t *testing.T) {
	spec := looperSpec()
	callbackSet := map[string]bool{"ALooper_callbackFunc": true}

	p := specmodel.Param{Name: "looper", Type: "*ALooper"}
	conv := paramConversion(p, spec, callbackSet, nil)

	assert.Contains(t, conv.code, "(*C.ALooper)(unsafe.Pointer(looper))")
	assert.Equal(t, "clooper", conv.cVarName)
}

func TestParamConversionScalar(t *testing.T) {
	spec := looperSpec()
	callbackSet := map[string]bool{}

	p := specmodel.Param{Name: "fd", Type: "int32"}
	conv := paramConversion(p, spec, callbackSet, nil)

	assert.Contains(t, conv.code, "(C.int)(fd)")
	assert.Equal(t, "cfd", conv.cVarName)
}

func TestParamConversionCallback(t *testing.T) {
	spec := looperSpec()
	callbackSet := map[string]bool{"ALooper_callbackFunc": true}

	p := specmodel.Param{Name: "callback", Type: "ALooper_callbackFunc"}
	conv := paramConversion(p, spec, callbackSet, nil)

	assert.Contains(t, conv.code, ".PassValue()")
	assert.Equal(t, "ccallback", conv.cVarName)
}

func TestParamConversionUnsafePointer(t *testing.T) {
	spec := looperSpec()
	callbackSet := map[string]bool{}

	p := specmodel.Param{Name: "data", Type: "unsafe.Pointer"}
	conv := paramConversion(p, spec, callbackSet, nil)

	assert.Contains(t, conv.code, "data, cgoAllocsUnknown")
	assert.Equal(t, "cdata", conv.cVarName)
}

func TestParamConversionMultiLevelPointer(t *testing.T) {
	spec := looperSpec()
	callbackSet := map[string]bool{}

	p := specmodel.Param{Name: "outPtr", Type: "**ALooper"}
	conv := paramConversion(p, spec, callbackSet, nil)

	// Must use a named variable, not an inline expression.
	assert.Equal(t, "coutPtr", conv.cVarName)
	// Must contain runtime.Pinner to pin the Go pointer.
	assert.Contains(t, conv.code, "runtime.Pinner")
	assert.Contains(t, conv.code, "Pin(outPtr)")
	// Must also pin the inner pointer to satisfy CGo's check.
	assert.Contains(t, conv.code, "Pin(unsafe.Pointer(*outPtr))")
	assert.Contains(t, conv.code, "defer")
	assert.Contains(t, conv.code, "Unpin()")
	// Must cast to the correct C type.
	assert.Contains(t, conv.code, "(**C.ALooper)(unsafe.Pointer(outPtr))")
	// Must have a KeepAlive name.
	assert.NotEmpty(t, conv.keepAliveName)
}

func TestParamConversionMultiLevelPointerScalar(t *testing.T) {
	spec := looperSpec()
	callbackSet := map[string]bool{}

	p := specmodel.Param{Name: "data", Type: "**uint8"}
	conv := paramConversion(p, spec, callbackSet, nil)

	assert.Equal(t, "cdata", conv.cVarName)
	assert.Contains(t, conv.code, "runtime.Pinner")
	assert.Contains(t, conv.code, "Pin(data)")
	assert.Contains(t, conv.code, "(**C.uint8_t)(unsafe.Pointer(data))")
}

func TestReturnConversionPointer(t *testing.T) {
	spec := looperSpec()
	result := returnConversion("*ALooper", spec)
	assert.Contains(t, result, "(*ALooper)(unsafe.Pointer(__ret))")
	assert.NotContains(t, result, "unsafe.Pointer(&__ret)", "must not take address of __ret (double dereference bug)")
	assert.Contains(t, result, "return __v")
}

func TestReturnConversionOpaquePointer(t *testing.T) {
	spec := &specmodel.Spec{
		Types: map[string]specmodel.TypeDef{
			"AFont": {
				Kind:   "opaque_ptr",
				CType:  "AFont",
				GoType: "*C.AFont",
			},
			"AFontMatcher": {
				Kind:   "opaque_ptr",
				CType:  "AFontMatcher",
				GoType: "*C.AFontMatcher",
			},
		},
		Functions: map[string]specmodel.FuncDef{
			"AFontMatcher_match": {
				CName: "AFontMatcher_match",
				Params: []specmodel.Param{
					{Name: "matcher", Type: "*AFontMatcher"},
				},
				Returns: "*AFont",
			},
		},
	}

	result := returnConversion("*AFont", spec)
	assert.Equal(t, "\t__v := (*AFont)(unsafe.Pointer(__ret))\n\treturn __v\n", result)
	assert.NotContains(t, result, "&__ret", "must use direct pointer cast, not address-of reinterpret")
}

func TestReturnConversionScalar(t *testing.T) {
	spec := looperSpec()
	result := returnConversion("int32", spec)
	assert.Contains(t, result, "(int32)(__ret)")
}
