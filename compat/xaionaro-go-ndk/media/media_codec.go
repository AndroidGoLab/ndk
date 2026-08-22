//go:build android && cgo

// Copyright 2018-2024 The gooid Authors. All rights reserved.
// Use of this source code is governed by a MIT-style license.

package media

// #cgo LDFLAGS: -lmediandk
// #include <sys/types.h>
// #include <media/NdkMediaCodec.h>
// #include <media/NdkMediaFormat.h>
import "C"

// MediaCodec is a non-owning view of an NDK AMediaCodec handle.
type MediaCodec C.AMediaCodec

// CPointer returns the underlying NDK handle without transferring ownership.
func (mc *MediaCodec) CPointer() *C.AMediaCodec {
	return (*C.AMediaCodec)(mc)
}

// SetParameters applies params to the codec.
func (mc *MediaCodec) SetParameters(params *MediaFormat) error {
	status := C.AMediaCodec_setParameters(mc.CPointer(), params.CPointer())
	return CMediaStatusToError(status)
}
