package specmodel_test

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/AndroidGoLab/ndk/tools/pkg/specmodel"
)

func TestSpecRoundTrip(t *testing.T) {
	spec := specmodel.Spec{
		Module:        "aaudio",
		SourcePackage: "github.com/AndroidGoLab/ndk/capi/aaudio",
		Types: map[string]specmodel.TypeDef{
			"AAudioStream": {
				Kind:   "opaque_ptr",
				CType:  "AAudioStream",
				GoType: "*C.AAudioStream",
			},
		},
		Enums: map[string][]specmodel.EnumValue{
			"Aaudio_result_t": {
				{Name: "AAUDIO_OK", Value: 0},
				{Name: "AAUDIO_ERROR_BASE", Value: -900},
			},
		},
		Functions: map[string]specmodel.FuncDef{
			"AAudio_createStreamBuilder": {
				CName: "AAudio_createStreamBuilder",
				Params: []specmodel.Param{
					{Name: "builder", Type: "**AAudioStreamBuilder", CType: "AAudioStreamBuilder**", Direction: "out"},
				},
				Returns: "Aaudio_result_t",
			},
		},
		Callbacks: map[string]specmodel.CallbackDef{
			"AAudioStream_dataCallback": {
				Params: []specmodel.Param{
					{Name: "stream", Type: "*AAudioStream"},
					{Name: "userData", Type: "unsafe.Pointer"},
				},
				Returns: "Aaudio_data_callback_result_t",
			},
		},
	}

	data, err := yaml.Marshal(&spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got specmodel.Spec
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Module != spec.Module {
		t.Errorf("module = %q, want %q", got.Module, spec.Module)
	}
	if got.SourcePackage != spec.SourcePackage {
		t.Errorf("source_package = %q, want %q", got.SourcePackage, spec.SourcePackage)
	}
	if len(got.Types) != 1 {
		t.Fatalf("types count = %d, want 1", len(got.Types))
	}
	td := got.Types["AAudioStream"]
	if td.Kind != "opaque_ptr" || td.CType != "AAudioStream" {
		t.Errorf("type = %+v", td)
	}
	if len(got.Enums["Aaudio_result_t"]) != 2 {
		t.Errorf("enums count = %d, want 2", len(got.Enums["Aaudio_result_t"]))
	}
	if len(got.Functions) != 1 {
		t.Errorf("functions count = %d, want 1", len(got.Functions))
	}
	fn := got.Functions["AAudio_createStreamBuilder"]
	if fn.Params[0].Direction != "out" {
		t.Errorf("param direction = %q, want out", fn.Params[0].Direction)
	}
	if fn.Params[0].CType != "AAudioStreamBuilder**" {
		t.Errorf("param C type = %q, want AAudioStreamBuilder**", fn.Params[0].CType)
	}
	if len(got.Callbacks) != 1 {
		t.Errorf("callbacks count = %d, want 1", len(got.Callbacks))
	}
}
