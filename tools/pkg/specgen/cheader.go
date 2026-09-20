package specgen

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AndroidGoLab/ndk/tools/pkg/specmodel"
)

// cFuncDeclRe matches a top-level C function declaration after comment stripping
// and line joining: RetType FuncName(Params);
// It captures: (1) return type, (2) function name, (3) params string.
var cFuncDeclRe = regexp.MustCompile(
	`(?m)^(\w[\w\s]*(?:\*\s*)?)\b([A-Z]\w+)\s*\(([^)]*)\)\s*(?:__\w+\s*\([^)]*\)\s*)*;`,
)

var (
	// Matches: typedef RetType (*Name)(Params);
	// Also handles: typedef RetType* (*Name)(Params);
	funcPtrTypedefRe = regexp.MustCompile(
		`typedef\s+(\w[\w\s\*]*?)\s*\(\s*\*\s*(\w+)\s*\)\s*\(([^)]*)\)\s*;`,
	)

	// Matches: typedef struct Name { ... } Name;
	namedStructRe = regexp.MustCompile(
		`(?s)typedef\s+struct\s+(\w+)\s*\{(.*?)\}\s*(\w+)\s*;`,
	)

	// Matches: typedef struct { ... } Name;
	anonStructRe = regexp.MustCompile(
		`(?s)typedef\s+struct\s*\{(.*?)\}\s*(\w+)\s*;`,
	)

	// Matches inline function pointer field: RetType (*name)(Params);
	// Also handles: RetType* (*name)(Params);
	inlineFuncPtrFieldRe = regexp.MustCompile(
		`^\s*(\w[\w\s\*]*?)\s*\(\s*\*\s*(\w+)\s*\)\s*\(([^)]*)\)\s*;`,
	)

	// Matches regular struct field: Type name;
	// Handles qualifiers like const, pointers, etc.
	dataFieldRe = regexp.MustCompile(
		`^\s*([\w][\w\s\*]*?)\s+(\w+)\s*;`,
	)
)

// ParseStructsFromDir reads all .h files in dir and extracts struct definitions.
func ParseStructsFromDir(dir string) (map[string]specmodel.StructDef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}

	result := make(map[string]specmodel.StructDef)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".h" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}

		for name, sd := range parseStructsFromSource(string(data)) {
			result[name] = sd
		}
	}

	return result, nil
}

// parseStructsFromSource extracts struct definitions from C header source text.
//
// Two passes:
//  1. Collect typedef'd function pointer signatures into a lookup map.
//  2. Parse typedef struct bodies, resolving fields against the function pointer map.
func parseStructsFromSource(source string) map[string]specmodel.StructDef {
	source = stripConditionalBlocks(source, "__ANDROID_VNDK__")
	funcPtrTypedefs := parseFuncPtrTypedefs(source)
	result := make(map[string]specmodel.StructDef)

	for _, m := range namedStructRe.FindAllStringSubmatch(source, -1) {
		name := m[1]
		body := m[2]
		fields := parseStructBody(body, funcPtrTypedefs)
		if len(fields) > 0 {
			result[name] = specmodel.StructDef{Fields: fields}
		}
	}

	for _, m := range anonStructRe.FindAllStringSubmatch(source, -1) {
		body := m[1]
		name := m[2]
		fields := parseStructBody(body, funcPtrTypedefs)
		if len(fields) > 0 {
			result[name] = specmodel.StructDef{Fields: fields}
		}
	}

	return result
}

// funcPtrTypedef holds the parsed return type and parameters
// of a C function pointer typedef.
type funcPtrTypedef struct {
	returnType string
	params     []specmodel.Param
}

// parseFuncPtrTypedefs extracts all typedef'd function pointers from source.
func parseFuncPtrTypedefs(source string) map[string]funcPtrTypedef {
	result := make(map[string]funcPtrTypedef)
	for _, m := range funcPtrTypedefRe.FindAllStringSubmatch(source, -1) {
		retType := strings.TrimSpace(m[1])
		name := m[2]
		paramsStr := m[3]
		result[name] = funcPtrTypedef{
			returnType: retType,
			params:     parseCParams(paramsStr),
		}
	}
	return result
}

// parseStructBody parses the fields between { } of a struct definition.
// It handles doc comments (single-line and multi-line), inline function pointer
// fields, typedef'd function pointer fields, and regular data fields.
func parseStructBody(
	body string,
	funcPtrTypedefs map[string]funcPtrTypedef,
) []specmodel.StructField {
	var fields []specmodel.StructField

	lines := strings.Split(body, "\n")
	inBlockComment := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Handle block comment start/end.
		if inBlockComment {
			if strings.Contains(trimmed, "*/") {
				inBlockComment = false
			}
			continue
		}

		if strings.HasPrefix(trimmed, "/*") {
			if !strings.Contains(trimmed, "*/") {
				inBlockComment = true
			}
			continue
		}

		// Skip single-line comments and empty lines.
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
			continue
		}

		// Try inline function pointer field first.
		if m := inlineFuncPtrFieldRe.FindStringSubmatch(line); m != nil {
			retType := strings.TrimSpace(m[1])
			fieldName := m[2]
			paramsStr := m[3]
			params := parseCParams(paramsStr)

			field := specmodel.StructField{
				Name:   fieldName,
				Type:   "func_ptr",
				Params: params,
			}
			if retType != "void" {
				field.Returns = normalizeCType(retType)
			}
			fields = append(fields, field)
			continue
		}

		// Try regular data field (which might be a typedef'd func ptr).
		if m := dataFieldRe.FindStringSubmatch(line); m != nil {
			typeName := strings.TrimSpace(m[1])
			fieldName := m[2]

			if fpt, ok := funcPtrTypedefs[typeName]; ok {
				field := specmodel.StructField{
					Name:   fieldName,
					Type:   "func_ptr",
					Params: fpt.params,
				}
				if fpt.returnType != "void" {
					field.Returns = normalizeCType(fpt.returnType)
				}
				fields = append(fields, field)
			} else {
				fields = append(fields, specmodel.StructField{
					Name: fieldName,
					Type: normalizeCType(typeName),
				})
			}
		}
	}

	return fields
}

// parseCParams parses a C function parameter list string into Param slices.
// Handles formats like:
//
//	"void* context, ACameraDevice* device, int error"
func parseCParams(paramsStr string) []specmodel.Param {
	paramsStr = strings.TrimSpace(paramsStr)
	if paramsStr == "" || paramsStr == "void" {
		return nil
	}

	var params []specmodel.Param
	for _, p := range strings.Split(paramsStr, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		param := parseSingleCParam(p)
		params = append(params, param)
	}
	return params
}

// parseSingleCParam parses a single C parameter declaration like "const ARect* rect"
// into a specmodel.Param with normalized Go-style type.
func parseSingleCParam(decl string) specmodel.Param {
	decl = strings.TrimSpace(decl)
	decl = stripNullabilityAnnotations(decl)

	// Split into tokens.
	tokens := strings.Fields(decl)
	if len(tokens) == 0 {
		return specmodel.Param{}
	}

	// Last token is the name (may have trailing *).
	name := tokens[len(tokens)-1]
	typeTokens := tokens[:len(tokens)-1]

	// Handle cases where the name has leading * (e.g., "*outSize") or
	// the pointer is attached to the name.
	nameStars := 0
	for strings.HasPrefix(name, "*") {
		nameStars++
		name = name[1:]
	}

	// Build up the type from remaining tokens, counting pointers.
	isConst := false
	ptrCount := nameStars
	var typeNameParts []string

	for _, tok := range typeTokens {
		if tok == "const" {
			isConst = true
			continue
		}

		// Count and strip trailing asterisks from type tokens.
		for strings.HasSuffix(tok, "*") {
			ptrCount++
			tok = tok[:len(tok)-1]
		}

		// Strip remaining "const" after pointer removal (e.g., "char* const**").
		if tok == "const" {
			isConst = true
			continue
		}

		if tok != "" {
			typeNameParts = append(typeNameParts, tok)
		}
	}

	typeName := strings.Join(typeNameParts, " ")
	cDeclarator := normalizeCDeclarator(typeTokens, nameStars)

	// Normalize to Go-style pointer notation.
	goType := typeName
	if ptrCount > 0 {
		if typeName == "void" {
			goType = "void*"
			ptrCount--
		}

		for range ptrCount {
			goType = "*" + goType
		}
	}

	// Handle "char**" → "**char" etc.
	if typeName == "char" && goType == "**char" {
		goType = "**char"
	}

	return specmodel.Param{
		Name:  name,
		Type:  goType,
		CType: cDeclarator,
		Const: isConst,
	}
}

// stripNullabilityAnnotations removes Clang nullability attributes from a C declaration.
func stripNullabilityAnnotations(s string) string {
	for _, annotation := range []string{
		"_Nonnull",
		"_Nullable",
		"_Null_unspecified",
		"__nonnull",
		"__nullable",
	} {
		s = strings.ReplaceAll(s, annotation, "")
	}
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

func normalizeCDeclarator(typeTokens []string, nameStars int) string {
	declarator := strings.TrimSpace(strings.Join(typeTokens, " "))
	declarator = strings.ReplaceAll(declarator, " *", "*")
	declarator = strings.ReplaceAll(declarator, "* ", "*")
	if declarator == "" {
		declarator = "void"
	}
	return declarator + strings.Repeat("*", nameStars)
}

// ParseFunctionsFromDir reads all .h files in dir and extracts function declarations.
func ParseFunctionsFromDir(dir string) (map[string]specmodel.FuncDef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}

	result := make(map[string]specmodel.FuncDef)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".h" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}

		for name, fd := range parseFunctionsFromSource(string(data)) {
			result[name] = fd
		}
	}

	return result, nil
}

// parseFunctionsFromSource extracts C function declarations from header source.
// It strips comments, joins multi-line declarations, then matches function
// declarations starting with an uppercase letter (NDK API convention).
func parseFunctionsFromSource(source string) map[string]specmodel.FuncDef {
	result := make(map[string]specmodel.FuncDef)

	cleaned := stripCComments(source)
	cleaned = stripConditionalBlocks(cleaned, "__ANDROID_VNDK__")
	joined := joinCLines(cleaned)

	for _, m := range cFuncDeclRe.FindAllStringSubmatch(joined, -1) {
		retTypeStr := strings.TrimSpace(m[1])
		funcName := m[2]
		paramsStr := m[3]

		// Skip typedef'd function pointers and macros.
		if strings.Contains(retTypeStr, "typedef") {
			continue
		}

		// Skip JNI infrastructure and non-NDK functions.
		if strings.HasPrefix(funcName, "JNI_") {
			continue
		}

		// Parse parameters using the existing C param parser.
		params := parseCFuncParams(paramsStr)

		// Convert C return type to spec Go type.
		goRetType := cTypeToGoType(retTypeStr)
		if goRetType == "void" {
			goRetType = ""
		}

		result[funcName] = specmodel.FuncDef{
			CName:   funcName,
			Params:  params,
			Returns: goRetType,
		}
	}

	return result
}

// parseCFuncParams parses a C function parameter list and converts types to
// Go/spec format for function definitions.
func parseCFuncParams(paramsStr string) []specmodel.Param {
	paramsStr = strings.TrimSpace(paramsStr)
	if paramsStr == "" || paramsStr == "void" {
		return nil
	}

	var params []specmodel.Param
	for _, p := range strings.Split(paramsStr, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		// Strip /*out*/ annotation.
		isOut := strings.Contains(p, "/*out*/")
		p = strings.ReplaceAll(p, "/*out*/", "")
		p = strings.TrimSpace(p)

		raw := parseSingleCParam(p)

		goType := cTypeToGoType(raw.Type)

		dir := ""
		if isOut || (isDoublePointerType(goType) && !raw.Const) {
			dir = "out"
		}

		params = append(params, specmodel.Param{
			Name:      raw.Name,
			Type:      goType,
			CType:     raw.CType,
			Direction: dir,
			Const:     raw.Const,
		})
	}
	return params
}

// isDoublePointerType returns true if the type string represents a double pointer.
func isDoublePointerType(t string) bool {
	return strings.HasPrefix(t, "**")
}

// cTypeToGoType converts a C type to the Go/spec type format used in function
// definitions. Handles both C notation (trailing *) and Go notation (leading *).
func cTypeToGoType(cType string) string {
	cType = strings.TrimSpace(cType)
	cType = strings.TrimPrefix(cType, "const ")

	// Count pointer indirections from both ends.
	stars := 0
	for strings.HasSuffix(cType, "*") {
		stars++
		cType = strings.TrimSpace(cType[:len(cType)-1])
	}
	for strings.HasPrefix(cType, "*") {
		stars++
		cType = cType[1:]
	}

	if cType == "void" {
		if stars == 0 {
			return "void"
		}
		if stars == 1 {
			return "unsafe.Pointer"
		}
		result := "unsafe.Pointer"
		for range stars - 1 {
			result = "*" + result
		}
		return result
	}

	goBase := cBaseTypeToGo(cType)
	result := goBase
	for range stars {
		result = "*" + result
	}
	return result
}

// cBaseTypeToGo converts a C base type name to its Go equivalent.
func cBaseTypeToGo(cType string) string {
	switch cType {
	case "char":
		return "byte"
	case "int32_t":
		return "int32"
	case "uint32_t":
		return "uint32"
	case "int64_t":
		return "int64"
	case "uint64_t":
		return "uint64"
	case "int16_t":
		return "int16"
	case "uint16_t":
		return "uint16"
	case "int8_t":
		return "int8"
	case "uint8_t":
		return "uint8"
	case "float":
		return "float32"
	case "double":
		return "float64"
	case "bool", "_Bool":
		return "bool"
	case "size_t":
		return "uint"
	case "uintptr_t":
		return "uintptr"
	default:
		// Capitalize first letter for C typedef types (e.g., camera_status_t → Camera_status_t).
		if len(cType) > 0 && cType[0] >= 'a' && cType[0] <= 'z' && strings.Contains(cType, "_") {
			return strings.ToUpper(cType[:1]) + cType[1:]
		}
		return cType
	}
}

// stripCComments removes both line (//) and block (/* */) comments from C source,
// except for /*out*/ annotations which are handled separately.
func stripCComments(source string) string {
	var b strings.Builder
	i := 0
	for i < len(source) {
		if i+1 < len(source) && source[i] == '/' && source[i+1] == '/' {
			// Line comment — skip to end of line.
			for i < len(source) && source[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(source) && source[i] == '/' && source[i+1] == '*' {
			// Check for /*out*/ annotation — preserve it.
			if i+6 < len(source) && source[i:i+7] == "/*out*/" {
				b.WriteString("/*out*/")
				i += 7
				continue
			}
			// Block comment — skip until closing */.
			i += 2
			closed := false
			for i+1 < len(source) {
				if source[i] == '*' && source[i+1] == '/' {
					i += 2
					closed = true
					break
				}
				i++
			}
			// If unterminated, consume all remaining bytes so the
			// outer loop doesn't leak the last byte as non-comment.
			if !closed {
				i = len(source)
			}
			continue
		}
		b.WriteByte(source[i])
		i++
	}
	return b.String()
}

// stripConditionalBlocks removes #ifdef MACRO ... #endif blocks from source.
// Handles nested #ifdef/#endif. Used to exclude vendor-only (VNDK) code.
func stripConditionalBlocks(source, macro string) string {
	lines := strings.Split(source, "\n")
	var result []string
	depth := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "#ifdef "+macro {
			depth++
			continue
		}
		if depth > 0 {
			switch {
			case strings.HasPrefix(trimmed, "#ifdef") || strings.HasPrefix(trimmed, "#ifndef"):
				depth++
			case trimmed == "#endif":
				depth--
			}
			continue
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

// joinCLines collapses multi-line C declarations into single lines.
// It joins any line that doesn't end with ';', '{', '}', or '#' (preprocessor)
// with the following line.
func joinCLines(source string) string {
	lines := strings.Split(source, "\n")
	var result []string
	var current strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
			continue
		}

		// Skip preprocessor directives.
		if strings.HasPrefix(trimmed, "#") {
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
			continue
		}

		if current.Len() > 0 {
			current.WriteByte(' ')
		}
		current.WriteString(trimmed)

		// If line ends with ; or { or }, flush.
		if strings.HasSuffix(trimmed, ";") || strings.HasSuffix(trimmed, "{") || strings.HasSuffix(trimmed, "}") {
			result = append(result, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return strings.Join(result, "\n")
}

// normalizeCType converts a C type string to a normalized form for the spec.
// Examples:
//
//	"int"         → "int"
//	"void*"       → "void*"
//	"const char**" → "**char"
func normalizeCType(cType string) string {
	cType = strings.TrimSpace(cType)

	tokens := strings.Fields(cType)
	ptrCount := 0
	var typeNameParts []string

	for _, tok := range tokens {
		if tok == "const" {
			continue
		}

		for strings.HasSuffix(tok, "*") {
			ptrCount++
			tok = tok[:len(tok)-1]
		}

		if tok != "" {
			typeNameParts = append(typeNameParts, tok)
		}
	}

	typeName := strings.Join(typeNameParts, " ")

	if ptrCount > 0 {
		if typeName == "void" {
			return "void*"
		}

		result := typeName
		for range ptrCount {
			result = "*" + result
		}
		return result
	}

	return typeName
}
