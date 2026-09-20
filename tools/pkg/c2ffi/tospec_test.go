package c2ffi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AndroidGoLab/ndk/tools/pkg/specmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertLooper(t *testing.T) {
	input := `[
		{"tag":"struct","ns":0,"name":"ALooper","id":0,"location":"android/looper.h:41:8","bit-size":0,"bit-alignment":0,"fields":[]},
		{"tag":"typedef","ns":0,"name":"ALooper","location":"android/looper.h:55:24","type":{"tag":"struct","ns":0,"name":"ALooper","id":0}},
		{"tag":"function","name":"ALooper_forThread","ns":0,"location":"android/looper.h:61:10","variadic":false,"inline":false,"storage-class":"none","parameters":[],"return-type":{"tag":":pointer","type":{"tag":"ALooper"}}},
		{"tag":"enum","ns":0,"name":"","id":3,"location":"android/looper.h:85:1","fields":[
			{"tag":"field","name":"ALOOPER_POLL_WAKE","value":4294967295},
			{"tag":"field","name":"ALOOPER_POLL_CALLBACK","value":4294967294}
		]},
		{"tag":"typedef","ns":0,"name":"ALooper_callbackFunc","location":"android/looper.h:179:15","type":{"tag":":function-pointer"}},
		{"tag":"function","name":"ALooper_addFd","ns":0,"location":"android/looper.h:275:5","variadic":false,"inline":false,"storage-class":"none","parameters":[
			{"tag":"parameter","name":"looper","type":{"tag":":pointer","type":{"tag":"ALooper"}}},
			{"tag":"parameter","name":"fd","type":{"tag":":int","bit-size":32,"bit-alignment":32}},
			{"tag":"parameter","name":"callback","type":{"tag":"ALooper_callbackFunc"}},
			{"tag":"parameter","name":"data","type":{"tag":":pointer","type":{"tag":":void"}}}
		],"return-type":{"tag":":int","bit-size":32,"bit-alignment":32}}
	]`

	opts := ConvertOptions{
		Module:        "looper",
		SourcePackage: "github.com/AndroidGoLab/ndk/capi/looper",
		TargetHeaders: []string{"android/looper.h"},
		Rules: []Rule{
			{Action: "accept", From: "^ALooper"},
			{Action: "accept", From: "^ALOOPER_"},
		},
	}

	spec, err := Convert([]byte(input), opts)
	require.NoError(t, err)

	// Type: ALooper → opaque_ptr.
	assert.Contains(t, spec.Types, "ALooper")
	assert.Equal(t, "opaque_ptr", spec.Types["ALooper"].Kind)

	// Functions.
	assert.Contains(t, spec.Functions, "ALooper_forThread")
	assert.Equal(t, "*ALooper", spec.Functions["ALooper_forThread"].Returns)

	assert.Contains(t, spec.Functions, "ALooper_addFd")
	fd := spec.Functions["ALooper_addFd"]
	assert.Equal(t, "int32", fd.Returns)
	assert.Len(t, fd.Params, 4)
	assert.Equal(t, "*ALooper", fd.Params[0].Type)
	assert.Equal(t, "int32", fd.Params[1].Type)
	assert.Equal(t, "ALooper_callbackFunc", fd.Params[2].Type)
	assert.Equal(t, "unsafe.Pointer", fd.Params[3].Type)

	// Enums: negative values via unsigned-to-signed conversion.
	assert.Contains(t, spec.Enums, "ALOOPER_POLL")
	pollVals := spec.Enums["ALOOPER_POLL"]
	assert.Len(t, pollVals, 2)
	assert.Equal(t, int64(-1), pollVals[0].Value)
	assert.Equal(t, int64(-2), pollVals[1].Value)

	// Callback (empty params — no header dirs for supplement).
	assert.Contains(t, spec.Callbacks, "ALooper_callbackFunc")
}

func TestConvertStructInlineUnionFields(t *testing.T) {
	input := `[
		{"tag":"struct","ns":0,"name":"ACameraMetadata_const_entry","id":1,"location":"camera/NdkCameraMetadata.h:143:16","bit-size":192,"bit-alignment":64,"fields":[
			{"tag":"field","name":"tag","type":{"tag":"uint32_t"}},
			{"tag":"field","name":"count","type":{"tag":"uint32_t"}},
			{"tag":"field","name":"data","type":{"tag":"union","fields":[
				{"tag":"field","name":"u8","type":{"tag":":pointer","type":{"tag":"uint8_t"}}},
				{"tag":"field","name":"i32","type":{"tag":":pointer","type":{"tag":"int32_t"}}},
				{"tag":"field","name":"f","type":{"tag":":pointer","type":{"tag":":float","bit-size":32,"bit-alignment":32}}}
			]}}
		]}
	]`

	opts := ConvertOptions{
		Module:        "camera",
		SourcePackage: "github.com/AndroidGoLab/ndk/capi/camera",
		TargetHeaders: []string{"camera/NdkCameraMetadata.h"},
	}

	spec, err := Convert([]byte(input), opts)
	require.NoError(t, err)

	entry, ok := spec.Structs["ACameraMetadata_const_entry"]
	require.True(t, ok)

	require.Len(t, entry.Fields, 3)
	data := entry.Fields[2]
	assert.Equal(t, "data", data.Name)
	assert.Equal(t, "union", data.Type)

	require.Len(t, data.Fields, 3)
	assert.Equal(t, specmodel.StructField{Name: "u8", Type: "*uint8"}, data.Fields[0])
	assert.Equal(t, specmodel.StructField{Name: "i32", Type: "*int32"}, data.Fields[1])
	assert.Equal(t, specmodel.StructField{Name: "f", Type: "*float32"}, data.Fields[2])
}

func TestConvertStructPreservesNestedCharCType(t *testing.T) {
	input := `[
		{"tag":"struct","name":"ACameraIdList","id":1,"location":"camera/NdkCameraDevice.h:60:16","bit-size":128,"bit-alignment":64,"fields":[
			{"tag":"field","name":"cameraIds","type":{"tag":":pointer","type":{"tag":":pointer","type":{"tag":":char"}}}}
		]}
	]`

	spec, err := Convert([]byte(input), ConvertOptions{Module: "camera"})
	require.NoError(t, err)
	field := spec.Structs["ACameraIdList"].Fields[0]
	assert.Equal(t, "**int8", field.Type)
	assert.Equal(t, "char**", field.CType)
}

func TestToSignedInt64(t *testing.T) {
	assert.Equal(t, int64(-1), toSignedInt64(4294967295))
	assert.Equal(t, int64(-4), toSignedInt64(4294967292))
	assert.Equal(t, int64(1), toSignedInt64(1))
	assert.Equal(t, int64(0), toSignedInt64(0))
	assert.Equal(t, int64(2147483647), toSignedInt64(2147483647))
}

func TestTypeRefToGoType(t *testing.T) {
	tests := []struct {
		name string
		ref  TypeRef
		want string
	}{
		{"void", TypeRef{Tag: ":void"}, ""},
		{"int32", TypeRef{Tag: ":int", BitSize: 32}, "int32"},
		{"uint32", TypeRef{Tag: ":unsigned-int", BitSize: 32}, "uint32"},
		{"int64", TypeRef{Tag: ":long", BitSize: 64}, "int64"},
		{"float32", TypeRef{Tag: ":float", BitSize: 32}, "float32"},
		{"float64", TypeRef{Tag: ":double", BitSize: 64}, "float64"},
		{"bool", TypeRef{Tag: ":_Bool"}, "bool"},
		{"void*", TypeRef{Tag: ":pointer", Type: &TypeRef{Tag: ":void"}}, "unsafe.Pointer"},
		{"char*", TypeRef{Tag: ":pointer", Type: &TypeRef{Tag: ":char"}}, "string"},
		{"char**", TypeRef{Tag: ":pointer", Type: &TypeRef{Tag: ":pointer", Type: &TypeRef{Tag: ":char"}}}, "**int8"},
		{"char***", TypeRef{Tag: ":pointer", Type: &TypeRef{Tag: ":pointer", Type: &TypeRef{Tag: ":pointer", Type: &TypeRef{Tag: ":char"}}}}, "***int8"},
		{"signed char**", TypeRef{Tag: ":pointer", Type: &TypeRef{Tag: ":pointer", Type: &TypeRef{Tag: ":signed-char"}}}, "**int8"},
		{"int8_t**", TypeRef{Tag: ":pointer", Type: &TypeRef{Tag: ":pointer", Type: &TypeRef{Tag: "int8_t"}}}, "**int8"},
		{"int*", TypeRef{Tag: ":pointer", Type: &TypeRef{Tag: ":int", BitSize: 32}}, "*int32"},
		{"typedef ref", TypeRef{Tag: "ALooper"}, "ALooper"},
		{"int32_t", TypeRef{Tag: "int32_t"}, "int32"},
		{"size_t", TypeRef{Tag: "size_t"}, "uint64"},
		{"func ptr", TypeRef{Tag: ":function-pointer"}, "unsafe.Pointer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := typeRefToGoType(&tt.ref)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTypeRefToCType(t *testing.T) {
	tests := []struct {
		name string
		ref  TypeRef
		want string
	}{
		{"char*", TypeRef{Tag: ":pointer", Type: &TypeRef{Tag: ":char"}}, "char*"},
		{"char**", TypeRef{Tag: ":pointer", Type: &TypeRef{Tag: ":pointer", Type: &TypeRef{Tag: ":char"}}}, "char**"},
		{"signed char**", TypeRef{Tag: ":pointer", Type: &TypeRef{Tag: ":pointer", Type: &TypeRef{Tag: ":signed-char"}}}, "signed char**"},
		{"int8_t**", TypeRef{Tag: ":pointer", Type: &TypeRef{Tag: ":pointer", Type: &TypeRef{Tag: "int8_t"}}}, "int8_t**"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, typeRefToCType(&tt.ref))
		})
	}
}

func TestConvertPreservesNestedPointerCType(t *testing.T) {
	input := `[
		{"tag":"function","name":"char_out","location":"test.h:1:1","variadic":false,"parameters":[{"tag":"parameter","name":"out","type":{"tag":":pointer","type":{"tag":":pointer","type":{"tag":":char"}}}}],"return-type":{"tag":":void"}},
		{"tag":"function","name":"signed_out","location":"test.h:2:1","variadic":false,"parameters":[{"tag":"parameter","name":"out","type":{"tag":":pointer","type":{"tag":":pointer","type":{"tag":":signed-char"}}}}],"return-type":{"tag":":void"}},
		{"tag":"function","name":"int8_out","location":"test.h:3:1","variadic":false,"parameters":[{"tag":"parameter","name":"out","type":{"tag":":pointer","type":{"tag":":pointer","type":{"tag":"int8_t"}}}}],"return-type":{"tag":":void"}}
	]`

	spec, err := Convert([]byte(input), ConvertOptions{Module: "test"})
	require.NoError(t, err)
	assert.Equal(t, "**int8", spec.Functions["char_out"].Params[0].Type)
	assert.Equal(t, "char**", spec.Functions["char_out"].Params[0].CType)
	assert.Equal(t, "signed char**", spec.Functions["signed_out"].Params[0].CType)
	assert.Equal(t, "int8_t**", spec.Functions["int8_out"].Params[0].CType)
}

func TestSupplementFunctionParamsUsesExplicitPointerDirections(t *testing.T) {
	dir := t.TempDir()
	header := `
void AInput(const char * const *values);
void AInputBaseConst(const char **values);
void AInputTriple(const char ***values);
void AOutput(/*out*/ char **values);
bool AMediaFormat_getString(AMediaFormat* format, const char *name, const char **out);
void AIBinder_dump(void* binder, int fd, const char **args, unsigned int numArgs);
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.h"), []byte(header), 0o644))

	input := `[
		{"tag":"function","name":"AInput","location":"test.h:1:1","variadic":false,"parameters":[{"tag":"parameter","name":"values","type":{"tag":":pointer","type":{"tag":":pointer","type":{"tag":":char"}}}}],"return-type":{"tag":":void"}},
		{"tag":"function","name":"AInputBaseConst","location":"test.h:2:1","variadic":false,"parameters":[{"tag":"parameter","name":"values","type":{"tag":":pointer","type":{"tag":":pointer","type":{"tag":":char"}}}}],"return-type":{"tag":":void"}},
		{"tag":"function","name":"AInputTriple","location":"test.h:3:1","variadic":false,"parameters":[{"tag":"parameter","name":"values","type":{"tag":":pointer","type":{"tag":":pointer","type":{"tag":":pointer","type":{"tag":":char"}}}}}],"return-type":{"tag":":void"}},
		{"tag":"function","name":"AOutput","location":"test.h:4:1","variadic":false,"parameters":[{"tag":"parameter","name":"values","type":{"tag":":pointer","type":{"tag":":pointer","type":{"tag":":char"}}}}],"return-type":{"tag":":void"}},
		{"tag":"function","name":"AMediaFormat_getString","location":"test.h:5:1","variadic":false,"parameters":[{"tag":"parameter","name":"format","type":{"tag":":pointer","type":{"tag":"AMediaFormat"}}},{"tag":"parameter","name":"name","type":{"tag":":pointer","type":{"tag":":char"}}},{"tag":"parameter","name":"out","type":{"tag":":pointer","type":{"tag":":pointer","type":{"tag":":char"}}}}],"return-type":{"tag":":_Bool"}},
		{"tag":"function","name":"AIBinder_dump","location":"test.h:6:1","variadic":false,"parameters":[{"tag":"parameter","name":"binder","type":{"tag":":pointer","type":{"tag":":void"}}},{"tag":"parameter","name":"fd","type":{"tag":":int","bit-size":32}},{"tag":"parameter","name":"args","type":{"tag":":pointer","type":{"tag":":pointer","type":{"tag":":char"}}}},{"tag":"parameter","name":"numArgs","type":{"tag":":unsigned-int","bit-size":32}}],"return-type":{"tag":":void"}}
	]`

	spec, err := Convert([]byte(input), ConvertOptions{Module: "test", NDKHeaderDirs: []string{dir}})
	require.NoError(t, err)
	assert.Equal(t, "", spec.Functions["AInput"].Params[0].Direction)
	assert.Equal(t, "const char*const*", spec.Functions["AInput"].Params[0].CType)
	assert.True(t, spec.Functions["AInput"].Params[0].Const)
	assert.Equal(t, "out", spec.Functions["AInputBaseConst"].Params[0].Direction)
	assert.Equal(t, "out", spec.Functions["AInputTriple"].Params[0].Direction)
	assert.Equal(t, "out", spec.Functions["AOutput"].Params[0].Direction)
	assert.Equal(t, "out", spec.Functions["AMediaFormat_getString"].Params[2].Direction)
	assert.Equal(t, "", spec.Functions["AIBinder_dump"].Params[2].Direction)
}

func TestParseCallbacksPreservesCTypeAndConst(t *testing.T) {
	source := `
typedef void (*TestCallback)(const char **names, const AThing* thing, signed char* bytes);
`

	callback := parseCallbacksFromSource(source)["TestCallback"]
	require.Len(t, callback.Params, 3)

	assert.Equal(t, "const char**", callback.Params[0].CType)
	assert.True(t, callback.Params[0].Const)
	assert.Equal(t, "const AThing*", callback.Params[1].CType)
	assert.True(t, callback.Params[1].Const)
	assert.Equal(t, "signed char*", callback.Params[2].CType)
	assert.False(t, callback.Params[2].Const)
}

func TestParseCallbacksPreservesReturnCTypeAndNullability(t *testing.T) {
	source := `
typedef char* _Nullable (*_Nonnull APersistableBundle_stringAllocator)(int32_t sizeBytes,
                                                                        void* _Nullable context);
typedef const char* (*Getter)(const void* data);
`

	callback := parseCallbacksFromSource(source)["APersistableBundle_stringAllocator"]
	require.Len(t, callback.Params, 2)
	assert.Equal(t, "*int8", callback.Returns)
	assert.Equal(t, "char*", callback.ReturnsCType)
	assert.Equal(t, "int32_t", callback.Params[0].CType)
	assert.Equal(t, "void*", callback.Params[1].CType)
	getter := parseCallbacksFromSource(source)["Getter"]
	assert.Equal(t, "*int8", getter.Returns)
	assert.Equal(t, "const char*", getter.ReturnsCType)
}
