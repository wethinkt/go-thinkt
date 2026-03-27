package llm

import "testing"

func TestLookupEmbeddingModel(t *testing.T) {
	spec, err := LookupModel("nomic-embed-text-v1.5")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Dim != 768 {
		t.Fatalf("expected dim=768, got %d", spec.Dim)
	}
	if spec.Kind != KindEmbedding {
		t.Fatalf("expected kind=embedding, got %s", spec.Kind)
	}
}

func TestLookupSummarizationModel(t *testing.T) {
	spec, err := LookupModel("qwen2.5-3b-instruct")
	if err != nil {
		t.Fatal(err)
	}
	if spec.NCtx != 4096 {
		t.Fatalf("expected nctx=4096, got %d", spec.NCtx)
	}
	if spec.Kind != KindGeneration {
		t.Fatalf("expected kind=generation, got %s", spec.Kind)
	}
}

func TestLookupUnknownModel(t *testing.T) {
	_, err := LookupModel("nonexistent-model")
	if err == nil {
		t.Fatal("expected error for unknown model")
	}
}

func TestListModels(t *testing.T) {
	models := ListModels()
	if len(models) < 3 {
		t.Fatalf("expected at least 3 known models, got %d", len(models))
	}
	hasEmbed := false
	hasGen := false
	for _, m := range models {
		if m.Kind == KindEmbedding {
			hasEmbed = true
		}
		if m.Kind == KindGeneration {
			hasGen = true
		}
	}
	if !hasEmbed || !hasGen {
		t.Fatal("expected both embedding and generation models")
	}
}
