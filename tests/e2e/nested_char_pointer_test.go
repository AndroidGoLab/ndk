//go:build e2e

package main

import (
	"testing"
	"unsafe"

	rawmedia "github.com/AndroidGoLab/ndk/capi/media"
	rawpb "github.com/AndroidGoLab/ndk/capi/persistablebundle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRawCAPICharPointerOutput(t *testing.T) {
	format := rawmedia.AMediaFormat_new()
	require.NotNil(t, format, "AMediaFormat_new")
	defer rawmedia.AMediaFormat_delete(format)

	rawmedia.AMediaFormat_setString(format, "ndk.e2e.char-pointer", "safe")
	slot := e2eAllocCharSlot()
	require.NotNil(t, slot, "char** output slot allocation")
	defer e2eFreeCharSlot(slot)

	out := (**int8)(unsafe.Pointer(slot))
	require.True(t, rawmedia.AMediaFormat_getString(format, "ndk.e2e.char-pointer", out))
	value, ok := e2eCharSlotValue(slot)
	require.True(t, ok, "AMediaFormat_getString output")
	assert.Equal(t, "safe", value)
}

func TestRawCAPICharPointerInputArray(t *testing.T) {
	values := e2eAllocHeaderValues()
	require.NotNil(t, values, "const char* const* input array allocation")
	defer e2eFreeHeaderValues(values)

	bundle := rawpb.APersistableBundle_new()
	require.NotNil(t, bundle, "APersistableBundle_new")
	defer rawpb.APersistableBundle_delete(bundle)

	rawpb.APersistableBundle_putStringVector(
		bundle,
		"ndk.e2e.values",
		(**int8)(unsafe.Pointer(values)),
		2,
	)
	assert.Equal(t, int32(1), rawpb.APersistableBundle_size(bundle))
}
