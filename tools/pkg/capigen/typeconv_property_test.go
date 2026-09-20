package capigen

import (
	"testing"
)

// TestGoTypeToCGoType_AllScalars verifies all Go scalar types have CGo mappings.
func TestGoTypeToCGoType_AllScalars(t *testing.T) {
	cases := map[string]string{
		"byte":    "C.char",
		"int8":    "C.int8_t",
		"uint8":   "C.uint8_t",
		"int16":   "C.int16_t",
		"uint16":  "C.uint16_t",
		"int32":   "C.int",
		"uint32":  "C.uint",
		"int64":   "C.int64_t",
		"uint64":  "C.uint64_t",
		"float32": "C.float",
		"float64": "C.double",
		"bool":    "C._Bool",
	}
	for goType, want := range cases {
		if got := goTypeToCGoType(goType); got != want {
			t.Errorf("goTypeToCGoType(%q) = %q, want %q", goType, got, want)
		}
	}
}

// TestGoTypeToCHeaderType_AllScalars verifies all Go scalar types have C header mappings.
func TestGoTypeToCHeaderType_AllScalars(t *testing.T) {
	cases := map[string]string{
		"byte":    "char",
		"int8":    "int8_t",
		"uint8":   "uint8_t",
		"int16":   "int16_t",
		"uint16":  "uint16_t",
		"int32":   "int",
		"uint32":  "unsigned int",
		"int64":   "int64_t",
		"uint64":  "uint64_t",
		"float32": "float",
		"float64": "double",
		"bool":    "_Bool",
	}
	for goType, want := range cases {
		if got := goTypeToCHeaderType(goType); got != want {
			t.Errorf("goTypeToCHeaderType(%q) = %q, want %q", goType, got, want)
		}
	}
}

// TestGoTypeToCGoExactType_Int32Difference verifies int32/uint32 use exact typedef.
func TestGoTypeToCGoExactType_Int32Difference(t *testing.T) {
	if got := goTypeToCGoExactType("int32"); got != "C.int32_t" {
		t.Errorf("goTypeToCGoExactType(int32) = %q, want C.int32_t", got)
	}
	if got := goTypeToCGoExactType("uint32"); got != "C.uint32_t" {
		t.Errorf("goTypeToCGoExactType(uint32) = %q, want C.uint32_t", got)
	}
	// Other types should fall through to goTypeToCGoType.
	if got := goTypeToCGoExactType("int64"); got != "C.int64_t" {
		t.Errorf("goTypeToCGoExactType(int64) = %q, want C.int64_t", got)
	}
}

// TestGoTypeToCHeaderType_Pointers verifies pointer type conversion.
func TestGoTypeToCHeaderType_Pointers(t *testing.T) {
	cases := []struct{ in_, want string }{
		{"*ALooper", "ALooper*"},
		{"**ALooper", "ALooper**"},
		{"*int32", "int*"},
		{"unsafe.Pointer", "void*"},
		{"string", "char*"},
		{"", "void"},
	}
	for _, tc := range cases {
		if got := goTypeToCHeaderType(tc.in_); got != tc.want {
			t.Errorf("goTypeToCHeaderType(%q) = %q, want %q", tc.in_, got, tc.want)
		}
	}
}

// TestApplyCGoStructPrefix_Idempotent verifies the function is idempotent.
func TestApplyCGoStructPrefix_Idempotent(t *testing.T) {
	prefixSet := map[string]bool{
		"__android_log_message": true,
		"AInputEvent":           true,
	}
	inputs := []string{
		"*C.__android_log_message",
		"C.AInputEvent",
		"*C.ALooper",
		"C.int",
	}
	for _, input := range inputs {
		once := applyCGoStructPrefix(input, prefixSet)
		twice := applyCGoStructPrefix(once, prefixSet)
		if once != twice {
			t.Errorf("applyCGoStructPrefix not idempotent for %q: %q -> %q", input, once, twice)
		}
	}
}

// TestApplyCGoStructPrefix_NoPrefixCollision verifies that a short name in the
// set does not corrupt a longer name not in the set. This tests the fix for
// the substring collision bug.
func TestApplyCGoStructPrefix_NoPrefixCollision(t *testing.T) {
	// "foo" is in the set, "foobar" is NOT.
	prefixSet := map[string]bool{"foo": true}

	// "C.foobar" should NOT become "C.struct_foobar".
	input := "C.foobar"
	got := applyCGoStructPrefix(input, prefixSet)
	if got != "C.foobar" {
		t.Errorf("prefix collision: applyCGoStructPrefix(%q, {foo}) = %q, want %q",
			input, got, "C.foobar")
	}

	// But "C.foo" should become "C.struct_foo".
	input2 := "*C.foo"
	got2 := applyCGoStructPrefix(input2, prefixSet)
	if got2 != "*C.struct_foo" {
		t.Errorf("applyCGoStructPrefix(%q, {foo}) = %q, want %q",
			input2, got2, "*C.struct_foo")
	}

	// Both in same expression.
	input3 := "*C.foo, C.foobar"
	got3 := applyCGoStructPrefix(input3, prefixSet)
	if got3 != "*C.struct_foo, C.foobar" {
		t.Errorf("applyCGoStructPrefix(%q, {foo}) = %q, want %q",
			input3, got3, "*C.struct_foo, C.foobar")
	}
}

// TestApplyCGoStructPrefix_BothInSet verifies correct behavior when both
// a short and long name are in the set.
func TestApplyCGoStructPrefix_BothInSet(t *testing.T) {
	prefixSet := map[string]bool{"foo": true, "foobar": true}

	got := applyCGoStructPrefix("*C.foo, C.foobar", prefixSet)
	if got != "*C.struct_foo, C.struct_foobar" {
		t.Errorf("applyCGoStructPrefix with both in set = %q, want %q",
			got, "*C.struct_foo, C.struct_foobar")
	}
}

// TestGoTypeBaseKind_Roundtrip verifies typedef_ prefix is correctly stripped.
func TestGoTypeBaseKind_Roundtrip(t *testing.T) {
	cases := map[string]string{
		"typedef_int32":  "int32",
		"typedef_uint32": "uint32",
		"typedef_int64":  "int64",
		"typedef_:union": ":union",
		"opaque_ptr":     "",
	}
	for kind, want := range cases {
		if got := goTypeBaseKind(kind); got != want {
			t.Errorf("goTypeBaseKind(%q) = %q, want %q", kind, got, want)
		}
	}
}
