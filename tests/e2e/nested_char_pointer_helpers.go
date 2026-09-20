//go:build e2e

package main

/*
#include <stdlib.h>
#include <string.h>

static char **ndk_e2e_alloc_char_slot(void) {
	return (char **)calloc(1, sizeof(char *));
}

static void ndk_e2e_free_char_slot(char **slot) {
	free(slot);
}

static char *ndk_e2e_char_slot_value(char **slot) {
	return slot == NULL ? NULL : *slot;
}

static char *ndk_e2e_copy_string(const char *value) {
	size_t size = strlen(value) + 1;
	char *copy = (char *)malloc(size);
	if (copy != NULL) {
		memcpy(copy, value, size);
	}
	return copy;
}

static char **ndk_e2e_alloc_header_values(void) {
	char **values = (char **)calloc(2, sizeof(char *));
	if (values == NULL) {
		return NULL;
	}
	values[0] = ndk_e2e_copy_string("X-NDK-E2E");
	values[1] = ndk_e2e_copy_string("nested-char-pointer");
	if (values[0] == NULL || values[1] == NULL) {
		free(values[0]);
		free(values[1]);
		free(values);
		return NULL;
	}
	return values;
}

static void ndk_e2e_free_header_values(char **values) {
	if (values == NULL) {
		return;
	}
	free(values[0]);
	free(values[1]);
	free(values);
}

*/
import "C"

import "unsafe"

func e2eAllocCharSlot() unsafe.Pointer {
	return unsafe.Pointer(C.ndk_e2e_alloc_char_slot())
}

func e2eFreeCharSlot(slot unsafe.Pointer) {
	C.ndk_e2e_free_char_slot((**C.char)(slot))
}

func e2eCharSlotValue(slot unsafe.Pointer) (string, bool) {
	value := C.ndk_e2e_char_slot_value((**C.char)(slot))
	if value == nil {
		return "", false
	}
	return C.GoString(value), true
}

func e2eAllocHeaderValues() unsafe.Pointer {
	return unsafe.Pointer(C.ndk_e2e_alloc_header_values())
}

func e2eFreeHeaderValues(values unsafe.Pointer) {
	C.ndk_e2e_free_header_values((**C.char)(values))
}
