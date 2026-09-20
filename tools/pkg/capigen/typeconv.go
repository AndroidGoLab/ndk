package capigen

import (
	"fmt"
	"sort"
	"strings"
)

// goTypeToCGoType converts a Go type string (from spec) to the CGo
// type expression used in function wrapper conversions.
func goTypeToCGoType(goType string) string {
	switch goType {
	case "byte":
		return "C.char"
	case "int8":
		return "C.int8_t"
	case "uint8":
		return "C.uint8_t"
	case "int16":
		return "C.int16_t"
	case "uint16":
		return "C.uint16_t"
	case "int32":
		return "C.int"
	case "uint32":
		return "C.uint"
	case "int64":
		return "C.int64_t"
	case "uint64":
		return "C.uint64_t"
	case "float32":
		return "C.float"
	case "float64":
		return "C.double"
	case "bool":
		return "C._Bool"
	case "unsafe.Pointer":
		return "unsafe.Pointer"
	case "":
		return ""
	}
	return "C." + goType
}

// goTypeToCGoExactType converts a Go type to the exact CGo typedef type.
// Unlike goTypeToCGoType which maps int32→C.int and uint32→C.uint for
// value conversions, this uses the exact C typedef names (C.int32_t, C.uint32_t)
// needed when casting pointer types where CGo enforces type identity.
func goTypeToCGoExactType(goType string) string {
	switch goType {
	case "int32":
		return "C.int32_t"
	case "uint32":
		return "C.uint32_t"
	}
	return goTypeToCGoType(goType)
}

// goTypeToCHeaderType converts a Go type string to its C header representation
// (for cgo_helpers.h declarations).
func goTypeToCHeaderType(goType string) string {
	switch goType {
	case "byte":
		return "char"
	case "int8":
		return "int8_t"
	case "uint8":
		return "uint8_t"
	case "int16":
		return "int16_t"
	case "uint16":
		return "uint16_t"
	case "int32":
		return "int"
	case "uint32":
		return "unsigned int"
	case "int64":
		return "int64_t"
	case "uint64":
		return "uint64_t"
	case "float32":
		return "float"
	case "float64":
		return "double"
	case "bool":
		return "_Bool"
	case "unsafe.Pointer":
		return "void*"
	case "string":
		return "char*"
	case "":
		return "void"
	}

	// Pointer to type: *ALooper → ALooper*, *int8 → int8_t*
	if strings.HasPrefix(goType, "*") {
		base := goType[1:]
		cBase := goTypeToCHeaderType(base) // recurse for nested pointers
		return cBase + "*"
	}

	return goType
}

// goTypeToCGoCallbackParam converts a Go type string to the CGo type
// used in //export callback proxy function parameters.
func goTypeToCGoCallbackParam(goType string) string {
	if goType == "unsafe.Pointer" {
		return "unsafe.Pointer"
	}
	if goType == "string" {
		return "*C.char"
	}

	// Count pointer depth.
	stars := 0
	base := goType
	for strings.HasPrefix(base, "*") {
		stars++
		base = base[1:]
	}

	if stars > 0 {
		// For scalar base types, use the CGo type.
		if isScalarGoType(base) {
			cgoBase := goTypeToCGoType(base)
			return strings.Repeat("*", stars) + cgoBase
		}
		// For named types (structs, etc.).
		return strings.Repeat("*", stars) + "C." + base
	}

	return goTypeToCGoType(goType)
}

// goTypeToCGoCallbackParamWithCType converts a callback parameter using its
// original C declarator when the Go type has lost ABI identity.
func goTypeToCGoCallbackParamWithCType(goType, cType string, structPrefixSet map[string]bool) string {
	if cType == "" {
		return applyCGoStructPrefix(goTypeToCGoCallbackParam(goType), structPrefixSet)
	}

	pointerDepth := strings.Count(cType, "*")
	if pointerDepth == 0 {
		if exactBase := cgoBaseTypeFromCType(cType, 0); exactBase != "" && exactBase != "C.void" {
			return applyCGoStructPrefix(exactBase, structPrefixSet)
		}
		return applyCGoStructPrefix(goTypeToCGoCallbackParam(goType), structPrefixSet)
	}

	base := cgoBaseTypeFromCType(cType, pointerDepth)
	if base == "" || base == "C.void" {
		return applyCGoStructPrefix(goTypeToCGoCallbackParam(goType), structPrefixSet)
	}

	return applyCGoStructPrefix(strings.Repeat("*", pointerDepth)+base, structPrefixSet)
}

func goTypeToCGoCallbackReturnWithCType(goType, cType string, structPrefixSet map[string]bool) string {
	return goTypeToCGoCallbackParamWithCType(goType, cType, structPrefixSet)
}

// goTypeToCHeaderCallbackParam returns the exact C declaration for a callback
// proxy parameter when header metadata is available.
func goTypeToCHeaderCallbackParam(goType, cType string, structPrefixSet map[string]bool) string {
	if cType != "" {
		return cType
	}
	return goTypeToCHeaderTypeWithStructPrefix(goType, structPrefixSet)
}

func goTypeToCHeaderCallbackReturn(goType, cType string, structPrefixSet map[string]bool) string {
	if cType != "" {
		return cType
	}
	return goTypeToCHeaderTypeWithStructPrefix(goType, structPrefixSet)
}

// cTypeWithoutQualifiers removes qualifiers that CGo cannot express in an
// exported Go callback declaration.
func cTypeWithoutQualifiers(cType string) string {
	for _, qualifier := range []string{"const", "volatile", "restrict"} {
		cType = strings.ReplaceAll(cType, qualifier, "")
	}
	for strings.Contains(cType, "  ") {
		cType = strings.ReplaceAll(cType, "  ", " ")
	}
	return strings.TrimSpace(cType)
}

// cgoCallbackParamToGo generates code to convert a CGo callback parameter
// to its Go equivalent. Uses exported Go type names.
func cgoCallbackParamToGo(cVarName string, goVarName string, goType string) string {
	if goType == "unsafe.Pointer" {
		return fmt.Sprintf("\t\t%s := (unsafe.Pointer)(unsafe.Pointer(%s))\n", goVarName, cVarName)
	}
	if goType == "string" {
		return fmt.Sprintf("\t\t%s := C.GoString(%s)\n", goVarName, cVarName)
	}
	exported := exportGoType(goType)
	if strings.HasPrefix(goType, "*") {
		return fmt.Sprintf("\t\t%s := (%s)(unsafe.Pointer(%s))\n", goVarName, exported, cVarName)
	}
	return fmt.Sprintf("\t\t%s := (%s)(%s)\n", goVarName, exported, cVarName)
}

// goToCGoReturn generates the CGo expression to convert a Go return value
// back to the CGo type for a callback proxy return.
func goToCGoReturn(goRetVar string, goRetType string) string {
	cgoType := goTypeToCGoCallbackParam(goRetType)
	return fmt.Sprintf("(%s)(%s)", cgoType, goRetVar)
}

func goToCGoReturnWithCType(goRetVar, goRetType, cType string, structPrefixSet map[string]bool) string {
	cgoType := goTypeToCGoCallbackReturnWithCType(goRetType, cType, structPrefixSet)
	if strings.HasPrefix(cgoType, "*") {
		return fmt.Sprintf("(%s)(unsafe.Pointer(%s))", cgoType, goRetVar)
	}
	return fmt.Sprintf("(%s)(%s)", cgoType, goRetVar)
}

// isScalarGoType returns true for Go scalar types (not pointers, slices, etc.).
func isScalarGoType(t string) bool {
	switch t {
	case "byte", "int8", "uint8", "int16", "uint16", "int32", "uint32",
		"int64", "uint64", "float32", "float64", "bool", "int", "uint":
		return true
	}
	return false
}

// isFixedArrayType returns true if t is a fixed-size array type like "[16]float32".
func isFixedArrayType(t string) bool {
	return len(t) > 2 && t[0] == '[' && t[1] != ']'
}

// applyCGoStructPrefix converts a CGo type expression to use C.struct_ prefix
// where needed. E.g., "*C.__android_log_message" → "*C.struct___android_log_message"
// when "__android_log_message" is in the structPrefixSet.
//
// Uses longest-match-first ordering so that "C.foobar" is not corrupted by a
// shorter prefix "C.foo" when only "foo" is in the set but "foobar" is not.
func applyCGoStructPrefix(cgoExpr string, structPrefixSet map[string]bool) string {
	// Sort names by length descending so longer names are replaced first,
	// preventing shorter prefixes from matching substrings of longer names.
	names := make([]string, 0, len(structPrefixSet))
	for name := range structPrefixSet {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return len(names[i]) > len(names[j])
	})

	for _, name := range names {
		old := "C." + name
		replacement := "C.struct_" + name
		// Replace only exact token matches: "C.name" must not be followed
		// by an alphanumeric or underscore character (which would indicate
		// a longer identifier).
		result := strings.Builder{}
		remaining := cgoExpr
		for {
			idx := strings.Index(remaining, old)
			if idx < 0 {
				result.WriteString(remaining)
				break
			}
			// Check that match is not a substring of a longer identifier.
			afterIdx := idx + len(old)
			if afterIdx < len(remaining) {
				ch := remaining[afterIdx]
				if ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
					// Part of a longer identifier — skip this match.
					result.WriteString(remaining[:afterIdx])
					remaining = remaining[afterIdx:]
					continue
				}
			}
			result.WriteString(remaining[:idx])
			result.WriteString(replacement)
			remaining = remaining[afterIdx:]
		}
		cgoExpr = result.String()
	}
	return cgoExpr
}

// goTypeToCHeaderTypeWithStructPrefix converts a Go type to its C header
// representation, applying struct_ prefix where needed.
func goTypeToCHeaderTypeWithStructPrefix(goType string, structPrefixSet map[string]bool) string {
	base := goTypeToCHeaderType(goType)
	// Check if the base (without pointer stars) is a struct-prefixed type.
	trimmed := strings.TrimRight(base, "*")
	trimmed = strings.TrimSpace(trimmed)
	if structPrefixSet[trimmed] {
		return "struct " + base
	}
	return base
}

// goTypeBaseKind returns the underlying Go scalar type for a typedef kind.
// e.g., "typedef_int32" → "int32".
func goTypeBaseKind(kind string) string {
	if !strings.HasPrefix(kind, "typedef_") {
		return ""
	}
	return strings.TrimPrefix(kind, "typedef_")
}
