package specmodel

// StructField is one field in a C struct.
type StructField struct {
	Name         string        `yaml:"name"`
	Type         string        `yaml:"type"`
	CType        string        `yaml:"c_type,omitempty"` // normalized C declarator when Go loses pointer identity
	Params       []Param       `yaml:"params,omitempty"`
	Returns      string        `yaml:"returns,omitempty"` // non-void return type for func_ptr fields
	ReturnsCType string        `yaml:"returns_c_type,omitempty"`
	Fields       []StructField `yaml:"fields,omitempty"` // nested fields for inline union/struct members
}
