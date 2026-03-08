package summarize

import (
	"testing"
)

func TestLookupModel(t *testing.T) {
	spec, err := LookupModel("qwen2.5-3b-instruct")
	if err != nil {
		t.Fatalf("LookupModel: %v", err)
	}
	if spec.ID != "qwen2.5-3b-instruct" {
		t.Errorf("ID = %q, want %q", spec.ID, "qwen2.5-3b-instruct")
	}
	if spec.NCtx != 4096 {
		t.Errorf("NCtx = %d, want 4096", spec.NCtx)
	}
}

func TestLookupModelDefault(t *testing.T) {
	spec, err := LookupModel("")
	if err != nil {
		t.Fatalf("LookupModel empty: %v", err)
	}
	if spec.ID != DefaultModelID {
		t.Errorf("default ID = %q, want %q", spec.ID, DefaultModelID)
	}
}

func TestLookupModelUnknown(t *testing.T) {
	_, err := LookupModel("nonexistent-model")
	if err == nil {
		t.Fatal("expected error for unknown model")
	}
}

func TestModelPathForID(t *testing.T) {
	p, err := ModelPathForID("qwen2.5-3b-instruct")
	if err != nil {
		t.Fatalf("ModelPathForID: %v", err)
	}
	if p == "" {
		t.Fatal("ModelPathForID returned empty string")
	}
	spec := KnownModels["qwen2.5-3b-instruct"]
	if got := p[len(p)-len(spec.FileName):]; got != spec.FileName {
		t.Errorf("path ends with %q, want %q", got, spec.FileName)
	}
}
