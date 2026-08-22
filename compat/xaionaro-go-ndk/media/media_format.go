//go:build android && cgo

// Copyright 2018-2024 The gooid Authors. All rights reserved.
// Use of this source code is governed by a MIT-style license.

package media

// #cgo LDFLAGS: -lmediandk
// #include <sys/types.h>
// #include <media/NdkMediaFormat.h>
import "C"

// MediaFormat is a non-owning view of an NDK AMediaFormat handle.
type MediaFormat C.AMediaFormat

// CPointer returns the underlying NDK handle without transferring ownership.
func (mf *MediaFormat) CPointer() *C.AMediaFormat {
	return (*C.AMediaFormat)(mf)
}
