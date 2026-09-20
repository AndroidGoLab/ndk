package c2ffi

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AndroidGoLab/ndk/tools/pkg/specgen"
	"github.com/AndroidGoLab/ndk/tools/pkg/specmodel"
)

// funcPtrTypedefRe matches: typedef RetType (*Name)(Params);
var funcPtrTypedefRe = regexp.MustCompile(
	`typedef\s+(\w[\w\s\*]*?)\s*\(\s*\*\s*(\w+)\s*\)\s*\(([^)]*)\)\s*;`,
)

// funcPtrFieldRe matches inline struct field function pointers:
//
//	RetType (*fieldName)(Params);
//
// It excludes typedef lines by using a negative lookbehind approximation:
// the match must start at the beginning of the (whitespace-trimmed) line
// and NOT begin with "typedef".
var funcPtrFieldRe = regexp.MustCompile(
	`(?m)^\s+(\w[\w\s\*]*?)\s*\(\s*\*\s*(\w+)\s*\)\s*\(([^)]*)\)\s*;`,
)

// supplementCallbacks fills in callback param/return info from C headers.
// c2ffi outputs function pointer typedefs as just ":function-pointer"
// without parameter details, so we extract them with regex.
func supplementCallbacks(
	spec *specmodel.Spec,
	headerDirs []string,
) {
	needsCallbacks := len(spec.Callbacks) > 0
	needsStructFields := false
	for _, sd := range spec.Structs {
		for _, f := range sd.Fields {
			if f.Type == "func_ptr" && len(f.Params) == 0 {
				needsStructFields = true
				break
			}
		}
		if needsStructFields {
			break
		}
	}

	if !needsCallbacks && !needsStructFields {
		return
	}

	parsed := parseCallbacksFromDirs(headerDirs)

	for name, cb := range spec.Callbacks {
		if len(cb.Params) > 0 || cb.Returns != "" {
			continue
		}
		if info, ok := parsed[name]; ok {
			spec.Callbacks[name] = info
		}
	}

	// Also resolve func_ptr struct fields using parsed callback info
	// (for typedef-based callback field types).
	for sname, sd := range spec.Structs {
		for i, f := range sd.Fields {
			if f.Type == "" || strings.HasPrefix(f.Type, "*") || isBuiltinGoType(f.Type) {
				continue
			}
			if info, ok := parsed[f.Type]; ok {
				sd.Fields[i].Type = "func_ptr"
				sd.Fields[i].Params = info.Params
				sd.Fields[i].Returns = info.Returns
				sd.Fields[i].ReturnsCType = info.ReturnsCType
			}
		}
		spec.Structs[sname] = sd
	}

	// Resolve inline func_ptr struct fields (those already marked as
	// func_ptr by addStruct but without params) using inline function
	// pointer signatures parsed from headers.
	if needsStructFields {
		inlineFields := parseInlineFuncPtrFieldsFromDirs(headerDirs)
		for sname, sd := range spec.Structs {
			for i, f := range sd.Fields {
				if f.Type != "func_ptr" || len(f.Params) > 0 {
					continue
				}
				if info, ok := inlineFields[f.Name]; ok {
					sd.Fields[i].Params = info.Params
					sd.Fields[i].Returns = info.Returns
					sd.Fields[i].ReturnsCType = info.ReturnsCType
				}
			}
			spec.Structs[sname] = sd
		}
	}
}

func supplementFunctionParams(
	spec *specmodel.Spec,
	headerDirs []string,
) {
	for _, dir := range headerDirs {
		funcs, err := specgen.ParseFunctionsFromDir(dir)
		if err != nil {
			continue
		}
		for name, headerFunc := range funcs {
			specFunc, ok := spec.Functions[name]
			if !ok {
				continue
			}
			if len(specFunc.Params) != len(headerFunc.Params) {
				continue
			}
			for idx := range specFunc.Params {
				if specFunc.Params[idx].Name != headerFunc.Params[idx].Name {
					continue
				}
				if specFunc.Params[idx].Type != headerFunc.Params[idx].Type &&
					(specFunc.Params[idx].CType == "" ||
						canonicalCType(specFunc.Params[idx].CType) != canonicalCType(headerFunc.Params[idx].CType)) {
					continue
				}
				if specFunc.Params[idx].CType != "" || needsHeaderCTypeMetadata(headerFunc.Params[idx].CType) {
					specFunc.Params[idx].CType = headerFunc.Params[idx].CType
				}
				if headerFunc.Params[idx].Direction != "" {
					specFunc.Params[idx].Direction = headerFunc.Params[idx].Direction
				} else if isExplicitInputParam(name, headerFunc.Params[idx].Name) ||
					hasConstInnerPointerQualifier(headerFunc.Params[idx].CType) {
					// A const qualifier between pointer levels makes the
					// pointed-to array an input. A base const qualifier (for
					// example const char**) does not: Android APIs such as
					// AMediaFormat_getString use that form for output.
					specFunc.Params[idx].Direction = ""
				}
				if headerFunc.Params[idx].Const {
					specFunc.Params[idx].Const = true
				}
			}
			spec.Functions[name] = specFunc
		}
	}
}

// inputParamOverrides records APIs whose const char** parameter is an input
// array even though the C declarator is otherwise indistinguishable from the
// output form used by other APIs.
var inputParamOverrides = map[string]map[string]bool{
	"AIBinder_dump": {"args": true},
}

func isExplicitInputParam(functionName, paramName string) bool {
	return inputParamOverrides[functionName][paramName]
}

func hasConstInnerPointerQualifier(cType string) bool {
	canonical := strings.ReplaceAll(cType, " ", "")
	return strings.Contains(canonical, "*const*")
}

func canonicalCType(cType string) string {
	cType = strings.ReplaceAll(cType, "const", "")
	cType = strings.ReplaceAll(cType, " ", "")
	return cType
}

func needsHeaderCTypeMetadata(cType string) bool {
	canonical := canonicalCType(cType)
	stars := strings.Count(canonical, "*")
	if stars < 2 {
		return false
	}
	base := strings.TrimSuffix(canonical, strings.Repeat("*", stars))
	switch base {
	case "char", "signedchar", "unsignedchar", "int8_t", "uint8_t":
		return true
	default:
		return false
	}
}

func isBuiltinGoType(t string) bool {
	switch t {
	case "int8", "uint8", "int16", "uint16",
		"int32", "uint32", "int64", "uint64",
		"float32", "float64", "bool",
		"unsafe.Pointer", "string", "func_ptr":
		return true
	}
	return false
}

// parseCallbacksFromDirs reads .h files from all dirs and extracts
// function pointer typedef signatures.
func parseCallbacksFromDirs(dirs []string) map[string]specmodel.CallbackDef {
	result := make(map[string]specmodel.CallbackDef)

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".h" {
				continue
			}

			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}

			for name, cb := range parseCallbacksFromSource(string(data)) {
				result[name] = cb
			}
		}
	}

	return result
}

// parseInlineFuncPtrFieldsFromDirs reads .h files and extracts inline
// function pointer struct field signatures (e.g., void (*onStart)(ANativeActivity* activity);).
func parseInlineFuncPtrFieldsFromDirs(dirs []string) map[string]specmodel.CallbackDef {
	result := make(map[string]specmodel.CallbackDef)

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".h" {
				continue
			}

			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}

			for name, cb := range parseInlineFuncPtrFieldsFromSource(string(data)) {
				result[name] = cb
			}
		}
	}

	return result
}

func parseInlineFuncPtrFieldsFromSource(source string) map[string]specmodel.CallbackDef {
	result := make(map[string]specmodel.CallbackDef)
	source = stripNullabilityAnnotations(source)

	for _, m := range funcPtrFieldRe.FindAllStringSubmatch(source, -1) {
		retType := strings.TrimSpace(m[1])
		name := m[2]
		paramsStr := m[3]

		// Skip typedef lines (funcPtrFieldRe may match them too).
		if retType == "typedef" || strings.HasPrefix(retType, "typedef ") {
			continue
		}

		cb := specmodel.CallbackDef{
			Returns:      cReturnTypeToGo(retType),
			ReturnsCType: normalizeCReturnType(retType),
		}

		cb.Params = append(cb.Params, parseCParamList(paramsStr)...)

		result[name] = cb
	}

	return result
}

func parseCallbacksFromSource(source string) map[string]specmodel.CallbackDef {
	result := make(map[string]specmodel.CallbackDef)
	source = stripNullabilityAnnotations(source)

	for _, m := range funcPtrTypedefRe.FindAllStringSubmatch(source, -1) {
		retType := strings.TrimSpace(m[1])
		name := m[2]
		paramsStr := m[3]

		cb := specmodel.CallbackDef{
			Returns:      cReturnTypeToGo(retType),
			ReturnsCType: normalizeCReturnType(retType),
		}

		cb.Params = append(cb.Params, parseCParamList(paramsStr)...)

		result[name] = cb
	}

	return result
}

func normalizeCReturnType(cType string) string {
	cType = stripNullabilityAnnotations(cType)
	cType = strings.Join(strings.Fields(cType), " ")
	cType = strings.ReplaceAll(cType, " *", "*")
	cType = strings.ReplaceAll(cType, "* ", "*")
	if cType == "void" {
		return ""
	}
	return cType
}

func cReturnTypeToGo(cType string) string {
	cType = strings.TrimSpace(cType)
	if cType == "void" {
		return ""
	}

	stars := 0
	for strings.HasSuffix(cType, "*") {
		stars++
		cType = strings.TrimSpace(cType[:len(cType)-1])
	}

	if cType == "void" && stars > 0 {
		result := "unsafe.Pointer"
		for i := 1; i < stars; i++ {
			result = "*" + result
		}
		return result
	}

	goBase := cBaseToGo(cType)
	for range stars {
		goBase = "*" + goBase
	}
	return goBase
}

func parseCParamList(paramsStr string) []specmodel.Param {
	paramsStr = strings.TrimSpace(paramsStr)
	if paramsStr == "" || paramsStr == "void" {
		return nil
	}

	var params []specmodel.Param
	for _, part := range strings.Split(paramsStr, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		params = append(params, parseSingleCParam(part))
	}
	return params
}

func parseSingleCParam(decl string) specmodel.Param {
	decl = strings.TrimSpace(decl)

	// Strip nullability annotations.
	decl = stripNullabilityAnnotations(decl)

	tokens := strings.Fields(decl)
	if len(tokens) == 0 {
		return specmodel.Param{}
	}

	name := tokens[len(tokens)-1]
	typeTokens := tokens[:len(tokens)-1]

	// Count pointer stars attached to name.
	nameStars := 0
	for strings.HasPrefix(name, "*") {
		nameStars++
		name = name[1:]
	}
	cDeclarator := normalizeCDeclarator(typeTokens, nameStars)

	// If the name is empty or looks like a type, generate a synthetic name.
	if name == "" {
		name = "p0"
	}

	// Sanitize name: if it's a Go keyword, prefix with underscore.
	name = sanitizeGoName(name)

	isConst := false
	stars := nameStars
	var typeParts []string
	for _, tok := range typeTokens {
		if tok == "const" || tok == "struct" {
			if tok == "const" {
				isConst = true
			}
			continue
		}
		for strings.HasSuffix(tok, "*") {
			stars++
			tok = tok[:len(tok)-1]
		}
		if tok == "const" {
			isConst = true
			continue
		}
		if tok != "" {
			typeParts = append(typeParts, tok)
		}
	}

	typeName := strings.Join(typeParts, " ")

	// Special case: const char* → string.
	if typeName == "char" && stars == 1 && isConst {
		return specmodel.Param{Name: name, Type: "string", CType: cDeclarator, Const: true}
	}

	goType := cBaseToGo(typeName)
	if stars > 0 && typeName == "void" {
		goType = "unsafe.Pointer"
		stars--
	}
	for range stars {
		goType = "*" + goType
	}

	return specmodel.Param{
		Name:  name,
		Type:  goType,
		CType: cDeclarator,
		Const: isConst,
	}
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

// stripNullabilityAnnotations removes Clang nullability attributes from a C declaration.
func stripNullabilityAnnotations(s string) string {
	s = strings.ReplaceAll(s, "_Nonnull", "")
	s = strings.ReplaceAll(s, "_Nullable", "")
	s = strings.ReplaceAll(s, "_Null_unspecified", "")
	// Also handle __attribute__ forms.
	s = strings.ReplaceAll(s, "__nonnull", "")
	s = strings.ReplaceAll(s, "__nullable", "")
	// Clean up double spaces.
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

// sanitizeGoName returns a safe Go identifier for a parameter name.
func sanitizeGoName(name string) string {
	switch name {
	case "type", "func", "var", "const", "import", "package",
		"return", "range", "map", "chan", "interface", "struct",
		"error", "string", "bool", "int", "uint", "byte", "rune",
		"select", "case", "default", "switch", "break", "continue",
		"for", "go", "goto", "if", "else", "defer", "fallthrough":
		return "_" + name
	}
	return name
}

func cBaseToGo(cType string) string {
	cType = strings.TrimSpace(cType)
	cType = strings.TrimPrefix(cType, "const ")

	switch cType {
	case "void":
		return ""
	case "int":
		return "int32"
	case "unsigned int", "unsigned":
		return "uint32"
	case "long":
		return "int64"
	case "unsigned long":
		return "uint64"
	case "short":
		return "int16"
	case "unsigned short":
		return "uint16"
	case "char", "signed char":
		return "int8"
	case "unsigned char":
		return "uint8"
	case "int8_t":
		return "int8"
	case "uint8_t":
		return "uint8"
	case "int16_t":
		return "int16"
	case "uint16_t":
		return "uint16"
	case "int32_t":
		return "int32"
	case "uint32_t":
		return "uint32"
	case "int64_t":
		return "int64"
	case "uint64_t":
		return "uint64"
	case "float":
		return "float32"
	case "double":
		return "float64"
	case "size_t":
		return "uint64"
	case "ssize_t":
		return "int64"
	case "bool", "_Bool":
		return "bool"
	}
	return cType
}
