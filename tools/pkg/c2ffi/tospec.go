package c2ffi

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"

	"github.com/AndroidGoLab/ndk/tools/pkg/specmodel"
)

// Manifest holds the relevant fields from capi/manifests/*.yaml.
type Manifest struct {
	Generator struct {
		PackageName string   `yaml:"PackageName"`
		Includes    []string `yaml:"Includes"`
	} `yaml:"GENERATOR"`
	Translator struct {
		Rules struct {
			Global []Rule `yaml:"global"`
		} `yaml:"Rules"`
	} `yaml:"TRANSLATOR"`
}

// Rule is one accept/ignore filter rule from the manifest.
type Rule struct {
	Action string `yaml:"action"`
	From   string `yaml:"from"`
}

// ConvertOptions configures the c2ffi JSON → spec YAML conversion.
type ConvertOptions struct {
	Module        string
	SourcePackage string

	// TargetHeaders limits extraction to declarations from these headers.
	// Each entry is a suffix match against the location path (e.g., "android/looper.h").
	TargetHeaders []string

	// Rules filters declarations by accept/ignore regex patterns.
	Rules []Rule

	// NDKHeaderDirs are directories to scan for callback typedef params
	// (c2ffi doesn't provide function pointer typedef parameters).
	NDKHeaderDirs []string

	// NDKSysroot is the NDK sysroot include directory for extracting
	// #define macro constants from original (pre-preprocessed) headers.
	NDKSysroot string
}

// ConvertFile reads a c2ffi JSON file and converts it to a specmodel.Spec.
func ConvertFile(
	path string,
	opts ConvertOptions,
) (*specmodel.Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading c2ffi JSON: %w", err)
	}

	return Convert(data, opts)
}

// Convert parses c2ffi JSON data and converts it to a specmodel.Spec.
func Convert(
	data []byte,
	opts ConvertOptions,
) (*specmodel.Spec, error) {
	var decls []Declaration
	if err := json.Unmarshal(data, &decls); err != nil {
		return nil, fmt.Errorf("parsing c2ffi JSON: %w", err)
	}

	spec := &specmodel.Spec{
		Module:        opts.Module,
		SourcePackage: opts.SourcePackage,
		Types:         make(map[string]specmodel.TypeDef),
		Enums:         make(map[string][]specmodel.EnumValue),
		Functions:     make(map[string]specmodel.FuncDef),
		Callbacks:     make(map[string]specmodel.CallbackDef),
		Structs:       make(map[string]specmodel.StructDef),
	}

	// Index enums by ID so typedef→enum linkage works.
	enumByID := map[int]*enumState{}

	// Compile filter rules.
	filters := compileRules(opts.Rules)

	for i := range decls {
		d := &decls[i]

		if !matchesTargetHeader(d.Location, opts.TargetHeaders) {
			continue
		}

		switch d.Tag {
		case "function":
			if !passesFilter(d.Name, filters) {
				continue
			}
			addFunction(spec, d)

		case "typedef":
			if !passesFilter(d.Name, filters) {
				continue
			}
			addTypedef(spec, d, enumByID)

		case "enum":
			es := addEnum(spec, d, filters)
			if es != nil {
				enumByID[d.ID] = es
			}

		case "struct":
			if d.Name == "" || d.BitSize == 0 {
				continue
			}
			if !passesFilter(d.Name, filters) {
				continue
			}
			addStruct(spec, d)
		}
	}

	// Supplement callback params from C headers.
	if len(opts.NDKHeaderDirs) > 0 {
		supplementCallbacks(spec, opts.NDKHeaderDirs)
		supplementFunctionParams(spec, opts.NDKHeaderDirs)
	}

	// Extract #define macro constants from original headers.
	if opts.NDKSysroot != "" && len(opts.TargetHeaders) > 0 {
		macros, err := ExtractMacros(opts.NDKSysroot, opts.TargetHeaders)
		if err != nil {
			return nil, fmt.Errorf("extracting macros: %w", err)
		}
		if len(macros) > 0 {
			spec.Macros = macros
		}
	}

	// Remove empty maps.
	if len(spec.Types) == 0 {
		spec.Types = nil
	}
	if len(spec.Enums) == 0 {
		spec.Enums = nil
	}
	if len(spec.Functions) == 0 {
		spec.Functions = nil
	}
	if len(spec.Callbacks) == 0 {
		spec.Callbacks = nil
	}
	if len(spec.Structs) == 0 {
		spec.Structs = nil
	}

	return spec, nil
}

type enumState struct {
	groupName string
	values    []specmodel.EnumValue
}

func addFunction(spec *specmodel.Spec, d *Declaration) {
	// Skip variadic functions (not callable from CGo).
	if d.Variadic {
		return
	}

	// Skip functions with va_list parameters (not usable from Go).
	for _, p := range d.Parameters {
		if isVaListType(&p.Type) {
			return
		}
	}

	fd := specmodel.FuncDef{
		CName:   d.Name,
		Returns: typeRefToGoType(d.ReturnType),
	}

	for _, p := range d.Parameters {
		goType := typeRefToGoType(&p.Type)
		dir := ""
		if isOutputParam(&p.Type) {
			dir = "out"
		}
		cType := ""
		if needsCTypeMetadata(&p.Type) {
			cType = typeRefToCType(&p.Type)
		}
		fd.Params = append(fd.Params, specmodel.Param{
			Name:      p.Name,
			Type:      goType,
			CType:     cType,
			Direction: dir,
		})
	}

	spec.Functions[d.Name] = fd
}

// isVaListType returns true if the type reference is a va_list variant.
func isVaListType(t *TypeRef) bool {
	if t == nil {
		return false
	}
	switch t.Tag {
	case "va_list", "__builtin_va_list", "__gnuc_va_list":
		return true
	}
	return false
}

func addTypedef(
	spec *specmodel.Spec,
	d *Declaration,
	enumByID map[int]*enumState,
) {
	if d.Type == nil {
		return
	}

	switch {
	case d.Type.Tag == ":function-pointer":
		// Function pointer typedef — add as callback with empty params.
		// Params will be supplemented from C headers later.
		spec.Callbacks[d.Name] = specmodel.CallbackDef{}

	case d.Type.Tag == ":enum":
		// Typedef wrapping an anonymous enum — link the enum group name.
		if es, ok := enumByID[d.Type.ID]; ok {
			if es.groupName == "" {
				es.groupName = d.Name
			}
			// Re-register under the typedef name.
			if _, exists := spec.Enums[d.Name]; !exists {
				spec.Enums[d.Name] = es.values
			}
			// Remove the auto-derived group name if different.
			if es.groupName != d.Name {
				delete(spec.Enums, es.groupName)
				es.groupName = d.Name
			}
		}
		// Also add as a type.
		spec.Types[d.Name] = specmodel.TypeDef{
			Kind:   "typedef_int32",
			CType:  d.Name,
			GoType: "int32",
		}

	case d.Type.Tag == "struct" || d.Type.Tag == ":struct":
		// Typedef to struct — opaque pointer.
		spec.Types[d.Name] = specmodel.TypeDef{
			Kind:   "opaque_ptr",
			CType:  d.Name,
			GoType: "*C." + d.Name,
		}

	case d.Type.Tag == ":pointer" && d.Type.Type != nil && d.Type.Type.Tag == ":void":
		// typedef void* X → pointer handle.
		spec.Types[d.Name] = specmodel.TypeDef{
			Kind:   "pointer_handle",
			CType:  d.Name,
			GoType: "unsafe.Pointer",
		}

	case d.Type.Tag == ":void":
		// typedef void X — skip, not representable in Go.

	default:
		// Scalar typedef.
		goType := typeRefToGoType(d.Type)
		if goType == "" {
			// Skip typedefs that resolve to void.
			break
		}
		kind := goTypeToKind(goType)
		spec.Types[d.Name] = specmodel.TypeDef{
			Kind:   kind,
			CType:  d.Name,
			GoType: goType,
		}
	}
}

func addEnum(
	spec *specmodel.Spec,
	d *Declaration,
	filters []compiledRule,
) *enumState {
	if len(d.Fields) == 0 {
		return nil
	}

	// Filter individual enum constants.
	var values []specmodel.EnumValue
	for _, f := range d.Fields {
		if !passesFilter(f.Name, filters) {
			continue
		}
		values = append(values, specmodel.EnumValue{
			Name:  f.Name,
			Value: toSignedInt64(f.Value),
		})
	}

	if len(values) == 0 {
		return nil
	}

	// Derive group name from constant name prefix.
	groupName := d.Name
	if groupName == "" {
		groupName = deriveEnumGroupName(values)
	}

	if groupName == "" {
		return nil
	}

	es := &enumState{groupName: groupName, values: values}
	spec.Enums[groupName] = values
	return es
}

func addStruct(spec *specmodel.Spec, d *Declaration) {
	sd := specmodel.StructDef{}

	for _, f := range d.Fields {
		if f.Tag != "field" || f.Type == nil {
			continue
		}

		sf := specmodel.StructField{
			Name: f.Name,
		}

		if f.Type.Tag == ":function-pointer" {
			// Mark as func_ptr; params/returns will be filled by
			// supplementCallbacks from the C header source.
			sf.Type = "func_ptr"
		} else if isInlineAggregate(f.Type) {
			sf.Type = f.Type.Tag
			sf.Fields = convertInlineAggregateFields(f.Type.Fields)
		} else {
			sf.Type = typeRefToGoType(f.Type)
			if needsCTypeMetadata(f.Type) {
				sf.CType = typeRefToCType(f.Type)
			}
		}

		sd.Fields = append(sd.Fields, sf)
	}

	if len(sd.Fields) > 0 {
		spec.Structs[d.Name] = sd
	}
}

func isInlineAggregate(t *TypeRef) bool {
	if t == nil {
		return false
	}
	switch t.Tag {
	case "union", ":union", "struct", ":struct":
		return len(t.Fields) > 0
	default:
		return false
	}
}

func convertInlineAggregateFields(fields []Field) []specmodel.StructField {
	result := make([]specmodel.StructField, 0, len(fields))
	for _, f := range fields {
		if f.Tag != "field" || f.Type == nil {
			continue
		}

		sf := specmodel.StructField{
			Name: f.Name,
		}
		if f.Type.Tag == ":function-pointer" {
			sf.Type = "func_ptr"
		} else if isInlineAggregate(f.Type) {
			sf.Type = f.Type.Tag
			sf.Fields = convertInlineAggregateFields(f.Type.Fields)
		} else {
			sf.Type = typeRefToGoType(f.Type)
			if needsCTypeMetadata(f.Type) {
				sf.CType = typeRefToCType(f.Type)
			}
		}
		result = append(result, sf)
	}
	return result
}

// typeRefToGoType converts a c2ffi TypeRef to a Go type string.
func typeRefToGoType(t *TypeRef) string {
	if t == nil {
		return ""
	}

	switch t.Tag {
	case ":void":
		return ""
	case ":int":
		return "int32"
	case ":unsigned-int":
		return "uint32"
	case ":long":
		return "int64"
	case ":unsigned-long":
		return "uint64"
	case ":short":
		return "int16"
	case ":unsigned-short":
		return "uint16"
	case ":char", ":signed-char":
		return "int8"
	case ":unsigned-char":
		return "uint8"
	case ":float":
		return "float32"
	case ":double":
		return "float64"
	case ":_Bool":
		return "bool"

	case ":pointer":
		if t.Type == nil {
			return "unsafe.Pointer"
		}
		if t.Type.Tag == ":void" {
			return "unsafe.Pointer"
		}
		if isCharTypeRef(t.Type) {
			return "string"
		}
		if pointerDepth(t) > 1 && isCharTypeRef(pointerBaseType(t)) {
			return strings.Repeat("*", pointerDepth(t)) + "int8"
		}
		inner := typeRefToGoType(t.Type)
		return "*" + inner

	case ":array":
		inner := typeRefToGoType(t.Type)
		if t.Size > 0 {
			return fmt.Sprintf("[%d]%s", t.Size, inner)
		}
		return "[]" + inner

	case ":function-pointer":
		return "unsafe.Pointer"

	case ":enum":
		return "int32"
	case ":struct", "struct":
		if t.Name != "" {
			return t.Name
		}
		return "unsafe.Pointer"
	}

	// Named type reference (typedef name like "ALooper", "int32_t", "camera_status_t").
	return resolveTypedefName(t.Tag)
}

// typeRefToCType returns the normalized C spelling represented by a c2ffi
// type. It is retained in function parameters where Go's type system loses
// the distinction between char, signed char, and int8_t.
func typeRefToCType(t *TypeRef) string {
	if t == nil {
		return ""
	}

	switch t.Tag {
	case ":void":
		return "void"
	case ":char":
		return "char"
	case ":signed-char":
		return "signed char"
	case ":unsigned-char":
		return "unsigned char"
	case ":int":
		return "int"
	case ":unsigned-int":
		return "unsigned int"
	case ":long":
		return "long"
	case ":unsigned-long":
		return "unsigned long"
	case ":short":
		return "short"
	case ":unsigned-short":
		return "unsigned short"
	case ":float":
		return "float"
	case ":double":
		return "double"
	case ":_Bool":
		return "_Bool"
	case ":pointer":
		if t.Type == nil {
			return "void*"
		}
		inner := typeRefToCType(t.Type)
		if inner == "" {
			inner = "void"
		}
		return inner + "*"
	case ":array":
		inner := typeRefToCType(t.Type)
		if t.Size > 0 {
			return fmt.Sprintf("%s[%d]", inner, t.Size)
		}
		return inner + "[]"
	case ":function-pointer":
		return "function-pointer"
	case ":enum", ":struct", "struct", ":union", "union":
		if t.Name != "" {
			return t.Name
		}
		return t.Tag
	default:
		return t.Tag
	}
}

func isCharTypeRef(t *TypeRef) bool {
	if t == nil {
		return false
	}
	return t.Tag == ":char" || t.Tag == ":signed-char"
}

func pointerBaseType(t *TypeRef) *TypeRef {
	for t != nil && t.Tag == ":pointer" {
		t = t.Type
	}
	return t
}

func pointerDepth(t *TypeRef) int {
	depth := 0
	for t != nil && t.Tag == ":pointer" {
		depth++
		t = t.Type
	}
	return depth
}

func needsCTypeMetadata(t *TypeRef) bool {
	if pointerDepth(t) < 2 {
		return false
	}
	base := pointerBaseType(t)
	if base == nil {
		return false
	}
	switch base.Tag {
	case ":char", ":signed-char", ":unsigned-char", "int8_t", "uint8_t":
		return true
	default:
		return false
	}
}

// resolveTypedefName converts a C typedef name to its Go equivalent.
func resolveTypedefName(name string) string {
	switch name {
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
	case "size_t":
		return "uint64"
	case "ssize_t":
		return "int64"
	case "intptr_t":
		return "int64"
	case "uintptr_t":
		return "uint64"
	}
	return name
}

// isOutputParam detects double pointers (output parameters).
func isOutputParam(t *TypeRef) bool {
	if t == nil || t.Tag != ":pointer" {
		return false
	}
	if t.Type != nil && t.Type.Tag == ":pointer" {
		return true
	}
	return false
}

// toSignedInt64 converts a uint64 enum value to signed int64.
// c2ffi outputs all enum values as unsigned, so -1 becomes 4294967295.
func toSignedInt64(v uint64) int64 {
	if v > math.MaxInt32 && v <= math.MaxUint32 {
		return int64(int32(v))
	}
	return int64(v)
}

// goTypeToKind converts a Go type string to a spec kind string.
func goTypeToKind(goType string) string {
	switch goType {
	case "int8":
		return "typedef_int8"
	case "uint8":
		return "typedef_uint8"
	case "int16":
		return "typedef_int16"
	case "uint16":
		return "typedef_uint16"
	case "int32":
		return "typedef_int32"
	case "uint32":
		return "typedef_uint32"
	case "int64":
		return "typedef_int64"
	case "uint64":
		return "typedef_uint64"
	case "float32":
		return "typedef_float32"
	case "float64":
		return "typedef_float64"
	case "bool":
		return "typedef_bool"
	default:
		return "typedef_" + goType
	}
}

// matchesTargetHeader checks if a location string contains one of the target headers.
func matchesTargetHeader(location string, headers []string) bool {
	if len(headers) == 0 {
		return true
	}
	for _, h := range headers {
		// Match both "/android/looper.h:41" and "android/looper.h:41" (test data).
		if strings.Contains(location, "/"+h+":") || strings.Contains(location, "/"+h+" ") ||
			strings.HasPrefix(location, h+":") || strings.HasPrefix(location, h+" ") {
			return true
		}
	}
	return false
}

type compiledRule struct {
	action string
	re     *regexp.Regexp
}

func compileRules(rules []Rule) []compiledRule {
	var compiled []compiledRule
	for _, r := range rules {
		re, err := regexp.Compile(r.From)
		if err != nil {
			continue
		}
		compiled = append(compiled, compiledRule{action: r.Action, re: re})
	}
	return compiled
}

// passesFilter checks if a name passes the accept/ignore rules.
// If no rules match, the name is rejected (default deny).
func passesFilter(name string, rules []compiledRule) bool {
	if len(rules) == 0 {
		return true
	}
	for _, r := range rules {
		if r.re.MatchString(name) {
			return r.action == "accept"
		}
	}
	return false
}

// deriveEnumGroupName computes a group name from the common prefix of
// enum constant names.
func deriveEnumGroupName(values []specmodel.EnumValue) string {
	if len(values) == 0 {
		return ""
	}
	if len(values) == 1 {
		name := values[0].Name
		parts := strings.Split(name, "_")
		if len(parts) > 2 {
			return strings.Join(parts[:len(parts)-2], "_")
		}
		if idx := strings.LastIndex(name, "_"); idx > 0 {
			return name[:idx]
		}
		return name
	}

	prefix := values[0].Name
	for _, v := range values[1:] {
		prefix = commonPrefix(prefix, v.Name)
	}
	prefix = strings.TrimRight(prefix, "_")
	return prefix
}

func commonPrefix(a, b string) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return a[:i]
		}
	}
	return a[:n]
}
