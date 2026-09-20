// Package specmodel defines the YAML spec schema produced by specgen and consumed by idiomgen.
package specmodel

// Spec is the top-level structure representing one NDK module's extracted API.
type Spec struct {
	Module        string                 `yaml:"module"`
	SourcePackage string                 `yaml:"source_package"`
	Types         map[string]TypeDef     `yaml:"types,omitempty"`
	Enums         map[string][]EnumValue `yaml:"enums,omitempty"`
	Functions     map[string]FuncDef     `yaml:"functions,omitempty"`
	Callbacks     map[string]CallbackDef `yaml:"callbacks,omitempty"`
	Structs       map[string]StructDef   `yaml:"structs,omitempty"`
	Macros        map[string]int64       `yaml:"macros,omitempty"`
}

// TypeDef describes an extracted Go type.
type TypeDef struct {
	Kind   string `yaml:"kind"` // opaque_ptr, typedef_int32, typedef_uint32, etc.
	CType  string `yaml:"c_type"`
	GoType string `yaml:"go_type"`
}

// EnumValue is one constant in an enum group.
type EnumValue struct {
	Name  string `yaml:"name"`
	Value int64  `yaml:"value"`
}

// FuncDef describes an extracted function.
type FuncDef struct {
	CName   string  `yaml:"c_name"`
	Params  []Param `yaml:"params,omitempty"`
	Returns string  `yaml:"returns"`
}

// Param describes a function or callback parameter.
type Param struct {
	Name      string `yaml:"name"`
	Type      string `yaml:"type"`
	CType     string `yaml:"c_type,omitempty"`    // normalized C declarator (e.g. char**)
	Direction string `yaml:"direction,omitempty"` // "out" for output params
	Const     bool   `yaml:"const,omitempty"`     // true if param has const qualifier
}

// CallbackDef describes an extracted callback function type.
type CallbackDef struct {
	Params       []Param `yaml:"params,omitempty"`
	Returns      string  `yaml:"returns"`
	ReturnsCType string  `yaml:"returns_c_type,omitempty"`
}
