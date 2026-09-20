package specgen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStructsFromDir(t *testing.T) {
	structs, err := ParseStructsFromDir("testdata/headers")
	require.NoError(t, err)

	// Should find structs from both header files.
	require.Contains(t, structs, "ACameraDevice_StateCallbacks")
	require.Contains(t, structs, "ACameraIdList")
	require.Contains(t, structs, "ANativeActivityCallbacks")
}

func TestParseStructsFromDir_NonexistentDir(t *testing.T) {
	_, err := ParseStructsFromDir("testdata/nonexistent")
	require.Error(t, err)
}

func TestParseStructsFromSource_TypedefFuncPtrFields(t *testing.T) {
	source := `
typedef void (*ACameraDevice_StateCallback)(void* context, ACameraDevice* device);
typedef void (*ACameraDevice_ErrorStateCallback)(void* context, ACameraDevice* device, int error);

typedef struct ACameraDevice_StateCallbacks {
    /// optional application context.
    void*                             context;

    /**
     * Called when a camera device is no longer available for use.
     *
     * <p>Any attempt to call API methods on this ACameraDevice will return
     * {@link ACAMERA_ERROR_CAMERA_DISCONNECTED}.</p>
     */
    ACameraDevice_StateCallback       onDisconnected;

    /**
     * Called when a camera device has encountered a serious error.
     */
    ACameraDevice_ErrorStateCallback  onError;
} ACameraDevice_StateCallbacks;
`
	structs := parseStructsFromSource(source)
	require.Contains(t, structs, "ACameraDevice_StateCallbacks")

	sd := structs["ACameraDevice_StateCallbacks"]
	require.Len(t, sd.Fields, 3)

	// Field 0: void* context
	assert.Equal(t, "context", sd.Fields[0].Name)
	assert.Equal(t, "void*", sd.Fields[0].Type)
	assert.Empty(t, sd.Fields[0].Params)

	// Field 1: onDisconnected (func_ptr via typedef lookup)
	assert.Equal(t, "onDisconnected", sd.Fields[1].Name)
	assert.Equal(t, "func_ptr", sd.Fields[1].Type)
	require.Len(t, sd.Fields[1].Params, 2)
	assert.Equal(t, "context", sd.Fields[1].Params[0].Name)
	assert.Equal(t, "void*", sd.Fields[1].Params[0].Type)
	assert.Equal(t, "device", sd.Fields[1].Params[1].Name)
	assert.Equal(t, "*ACameraDevice", sd.Fields[1].Params[1].Type)

	// Field 2: onError (func_ptr via typedef lookup)
	assert.Equal(t, "onError", sd.Fields[2].Name)
	assert.Equal(t, "func_ptr", sd.Fields[2].Type)
	require.Len(t, sd.Fields[2].Params, 3)
	assert.Equal(t, "context", sd.Fields[2].Params[0].Name)
	assert.Equal(t, "device", sd.Fields[2].Params[1].Name)
	assert.Equal(t, "error", sd.Fields[2].Params[2].Name)
	assert.Equal(t, "int", sd.Fields[2].Params[2].Type)
}

func TestParseStructsFromSource_DataFields(t *testing.T) {
	source := `
typedef struct ACameraIdList {
    int numCameras;          ///< Number of camera device Ids
    const char** cameraIds;  ///< list of camera device Ids
} ACameraIdList;
`
	structs := parseStructsFromSource(source)
	require.Contains(t, structs, "ACameraIdList")

	sd := structs["ACameraIdList"]
	require.Len(t, sd.Fields, 2)

	assert.Equal(t, "numCameras", sd.Fields[0].Name)
	assert.Equal(t, "int", sd.Fields[0].Type)

	assert.Equal(t, "cameraIds", sd.Fields[1].Name)
	assert.Equal(t, "**char", sd.Fields[1].Type)
}

func TestParseStructsFromSource_InlineFuncPtrFields(t *testing.T) {
	source := `
typedef struct ANativeActivityCallbacks {
    /**
     * NativeActivity has started.
     */
    void (*onStart)(ANativeActivity* activity);

    /**
     * Save instance state callback.
     */
    void* (*onSaveInstanceState)(ANativeActivity* activity, size_t* outSize);

    /**
     * Window focus changed.
     */
    void (*onWindowFocusChanged)(ANativeActivity* activity, int hasFocus);

    /**
     * Native window created.
     */
    void (*onNativeWindowCreated)(ANativeActivity* activity, ANativeWindow* window);

    /**
     * Content rect changed.
     */
    void (*onContentRectChanged)(ANativeActivity* activity, const ARect* rect);
} ANativeActivityCallbacks;
`
	structs := parseStructsFromSource(source)
	require.Contains(t, structs, "ANativeActivityCallbacks")

	sd := structs["ANativeActivityCallbacks"]
	require.Len(t, sd.Fields, 5)

	// onStart
	assert.Equal(t, "onStart", sd.Fields[0].Name)
	assert.Equal(t, "func_ptr", sd.Fields[0].Type)
	require.Len(t, sd.Fields[0].Params, 1)
	assert.Equal(t, "activity", sd.Fields[0].Params[0].Name)
	assert.Equal(t, "*ANativeActivity", sd.Fields[0].Params[0].Type)

	// onSaveInstanceState
	assert.Equal(t, "onSaveInstanceState", sd.Fields[1].Name)
	assert.Equal(t, "func_ptr", sd.Fields[1].Type)
	require.Len(t, sd.Fields[1].Params, 2)
	assert.Equal(t, "activity", sd.Fields[1].Params[0].Name)
	assert.Equal(t, "*ANativeActivity", sd.Fields[1].Params[0].Type)
	assert.Equal(t, "outSize", sd.Fields[1].Params[1].Name)
	assert.Equal(t, "*size_t", sd.Fields[1].Params[1].Type)

	// onWindowFocusChanged
	assert.Equal(t, "onWindowFocusChanged", sd.Fields[2].Name)
	assert.Equal(t, "func_ptr", sd.Fields[2].Type)
	require.Len(t, sd.Fields[2].Params, 2)
	assert.Equal(t, "hasFocus", sd.Fields[2].Params[1].Name)
	assert.Equal(t, "int", sd.Fields[2].Params[1].Type)

	// onNativeWindowCreated
	assert.Equal(t, "onNativeWindowCreated", sd.Fields[3].Name)
	assert.Equal(t, "func_ptr", sd.Fields[3].Type)
	require.Len(t, sd.Fields[3].Params, 2)
	assert.Equal(t, "window", sd.Fields[3].Params[1].Name)
	assert.Equal(t, "*ANativeWindow", sd.Fields[3].Params[1].Type)

	// onContentRectChanged
	assert.Equal(t, "onContentRectChanged", sd.Fields[4].Name)
	assert.Equal(t, "func_ptr", sd.Fields[4].Type)
	require.Len(t, sd.Fields[4].Params, 2)
	assert.Equal(t, "rect", sd.Fields[4].Params[1].Name)
	assert.Equal(t, "*ARect", sd.Fields[4].Params[1].Type)
}

func TestParseStructsFromSource_AnonymousStruct(t *testing.T) {
	source := `
typedef struct {
    int width;
    int height;
} ASize;
`
	structs := parseStructsFromSource(source)
	require.Contains(t, structs, "ASize")

	sd := structs["ASize"]
	require.Len(t, sd.Fields, 2)
	assert.Equal(t, "width", sd.Fields[0].Name)
	assert.Equal(t, "int", sd.Fields[0].Type)
	assert.Equal(t, "height", sd.Fields[1].Name)
	assert.Equal(t, "int", sd.Fields[1].Type)
}

func TestParseStructsFromSource_EmptyStruct(t *testing.T) {
	source := `
typedef struct EmptyThing {
} EmptyThing;
`
	structs := parseStructsFromSource(source)
	assert.NotContains(t, structs, "EmptyThing")
}

func TestParseCParams(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLen  int
		wantName string
		wantType string
	}{
		{"empty", "", 0, "", ""},
		{"void", "void", 0, "", ""},
		{
			"simple int",
			"int error",
			1, "error", "int",
		},
		{
			"pointer type",
			"ACameraDevice* device",
			1, "device", "*ACameraDevice",
		},
		{
			"void pointer",
			"void* context",
			1, "context", "void*",
		},
		{
			"const pointer",
			"const ARect* rect",
			1, "rect", "*ARect",
		},
		{
			"pointer attached to name",
			"size_t* outSize",
			1, "outSize", "*size_t",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := parseCParams(tt.input)
			assert.Len(t, params, tt.wantLen)
			if tt.wantLen > 0 {
				assert.Equal(t, tt.wantName, params[0].Name)
				assert.Equal(t, tt.wantType, params[0].Type)
			}
		})
	}
}

func TestParseCParams_Multiple(t *testing.T) {
	params := parseCParams("void* context, ACameraDevice* device, int error")
	require.Len(t, params, 3)

	assert.Equal(t, "context", params[0].Name)
	assert.Equal(t, "void*", params[0].Type)

	assert.Equal(t, "device", params[1].Name)
	assert.Equal(t, "*ACameraDevice", params[1].Type)

	assert.Equal(t, "error", params[2].Name)
	assert.Equal(t, "int", params[2].Type)
}

func TestParseFunctionsFromSource_SingleLine(t *testing.T) {
	source := `
ACameraManager* ACameraManager_create() __INTRODUCED_IN(24);
void ACameraManager_delete(ACameraManager* manager) __INTRODUCED_IN(24);
`
	funcs := parseFunctionsFromSource(source)

	require.Contains(t, funcs, "ACameraManager_create")
	fd := funcs["ACameraManager_create"]
	assert.Equal(t, "ACameraManager_create", fd.CName)
	assert.Empty(t, fd.Params)
	assert.Equal(t, "*ACameraManager", fd.Returns)

	require.Contains(t, funcs, "ACameraManager_delete")
	fd = funcs["ACameraManager_delete"]
	assert.Equal(t, "ACameraManager_delete", fd.CName)
	require.Len(t, fd.Params, 1)
	assert.Equal(t, "manager", fd.Params[0].Name)
	assert.Equal(t, "*ACameraManager", fd.Params[0].Type)
	assert.Equal(t, "", fd.Returns)
}

func TestParseFunctionsFromSource_MultiLine(t *testing.T) {
	source := `
/**
 * Gets camera characteristics.
 */
camera_status_t ACameraManager_getCameraCharacteristics(
        ACameraManager* manager, const char* cameraId,
        /*out*/ACameraMetadata** characteristics) __INTRODUCED_IN(24);
`
	funcs := parseFunctionsFromSource(source)

	require.Contains(t, funcs, "ACameraManager_getCameraCharacteristics")
	fd := funcs["ACameraManager_getCameraCharacteristics"]
	assert.Equal(t, "ACameraManager_getCameraCharacteristics", fd.CName)
	assert.Equal(t, "Camera_status_t", fd.Returns)

	require.Len(t, fd.Params, 3)
	assert.Equal(t, "manager", fd.Params[0].Name)
	assert.Equal(t, "*ACameraManager", fd.Params[0].Type)

	assert.Equal(t, "cameraId", fd.Params[1].Name)
	assert.Equal(t, "*byte", fd.Params[1].Type)

	assert.Equal(t, "characteristics", fd.Params[2].Name)
	assert.Equal(t, "**ACameraMetadata", fd.Params[2].Type)
	assert.Equal(t, "out", fd.Params[2].Direction)
}

func TestParseFunctionsFromSource_SkipsStaticInline(t *testing.T) {
	source := `
static inline int helper_func(int x) { return x + 1; }
ACameraManager* ACameraManager_create(void);
`
	funcs := parseFunctionsFromSource(source)

	assert.NotContains(t, funcs, "helper_func")
	assert.Contains(t, funcs, "ACameraManager_create")
}

func TestParseFunctionsFromSource_IntTypes(t *testing.T) {
	source := `
camera_status_t ACameraMetadata_getConstEntry(
        const ACameraMetadata* metadata,
        uint32_t tag, /*out*/ACameraMetadata_const_entry* entry) __INTRODUCED_IN(24);
`
	funcs := parseFunctionsFromSource(source)

	require.Contains(t, funcs, "ACameraMetadata_getConstEntry")
	fd := funcs["ACameraMetadata_getConstEntry"]

	require.Len(t, fd.Params, 3)
	assert.Equal(t, "metadata", fd.Params[0].Name)
	assert.Equal(t, "*ACameraMetadata", fd.Params[0].Type)

	assert.Equal(t, "tag", fd.Params[1].Name)
	assert.Equal(t, "uint32", fd.Params[1].Type)

	assert.Equal(t, "entry", fd.Params[2].Name)
	assert.Equal(t, "*ACameraMetadata_const_entry", fd.Params[2].Type)
	assert.Equal(t, "out", fd.Params[2].Direction)
}

func TestParseFunctionsFromSource_ConstPointerArrayIsInput(t *testing.T) {
	source := `
void AInput(const char * const *values);
void AOutput(/*out*/ char **values);
`
	funcs := parseFunctionsFromSource(source)

	input := funcs["AInput"].Params[0]
	assert.Equal(t, "**byte", input.Type)
	assert.Equal(t, "const char*const*", input.CType)
	assert.True(t, input.Const)
	assert.Empty(t, input.Direction)

	output := funcs["AOutput"].Params[0]
	assert.Equal(t, "**byte", output.Type)
	assert.Equal(t, "char**", output.CType)
	assert.Equal(t, "out", output.Direction)
}

func TestParseFunctionsFromSource_StripsParameterNullability(t *testing.T) {
	source := `
void ANullable(const char* _Nonnull value, int* _Nullable out);
`

	params := parseFunctionsFromSource(source)["ANullable"].Params
	require.Len(t, params, 2)

	assert.Equal(t, "*byte", params[0].Type)
	assert.Equal(t, "const char*", params[0].CType)
	assert.Equal(t, "*int", params[1].Type)
	assert.Equal(t, "int*", params[1].CType)
}

func TestCTypeToGoType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"void", "void"},
		{"void*", "unsafe.Pointer"},
		{"*char", "*byte"},
		{"**char", "**byte"},
		{"*ACameraManager", "*ACameraManager"},
		{"**ACameraMetadata", "**ACameraMetadata"},
		{"int32_t", "int32"},
		{"uint32_t", "uint32"},
		{"camera_status_t", "Camera_status_t"},
		{"int", "int"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := cTypeToGoType(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeCType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"int", "int"},
		{"void*", "void*"},
		{"const char**", "**char"},
		{"ACameraDevice*", "*ACameraDevice"},
		{"const ARect*", "*ARect"},
		{"size_t", "size_t"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeCType(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
