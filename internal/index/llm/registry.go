package llm

import "fmt"

// ModelKind distinguishes embedding models from generative models.
type ModelKind string

const (
	KindEmbedding  ModelKind = "embedding"
	KindGeneration ModelKind = "generation"
)

// ModelSpec describes a known local model.
type ModelSpec struct {
	ID       string
	Kind     ModelKind
	FileName string
	URL      string
	Dim      int    // embedding dimension (0 for generation models)
	NCtx     uint32 // context window in tokens
}

// KnownModels maps model IDs to their specifications.
var KnownModels = map[string]ModelSpec{
	"nomic-embed-text-v1.5": {
		ID:       "nomic-embed-text-v1.5",
		Kind:     KindEmbedding,
		FileName: "nomic-embed-text-v1.5.Q8_0.gguf",
		URL:      "https://huggingface.co/nomic-ai/nomic-embed-text-v1.5-GGUF/resolve/main/nomic-embed-text-v1.5.Q8_0.gguf",
		Dim:      768,
		NCtx:     2048,
	},
	"qwen3-embedding-0.6b": {
		ID:       "qwen3-embedding-0.6b",
		Kind:     KindEmbedding,
		FileName: "Qwen3-Embedding-0.6B-Q8_0.gguf",
		URL:      "https://huggingface.co/Qwen/Qwen3-Embedding-0.6B-GGUF/resolve/main/Qwen3-Embedding-0.6B-Q8_0.gguf",
		Dim:      1024,
		NCtx:     2048,
	},
	"qwen2.5-3b-instruct": {
		ID:       "qwen2.5-3b-instruct",
		Kind:     KindGeneration,
		FileName: "Qwen2.5-3B-Instruct-Q4_K_M.gguf",
		URL:      "https://huggingface.co/Qwen/Qwen2.5-3B-Instruct-GGUF/resolve/main/qwen2.5-3b-instruct-q4_k_m.gguf",
		NCtx:     4096,
	},
}

// LookupModel returns the ModelSpec for a given model ID.
func LookupModel(modelID string) (ModelSpec, error) {
	spec, ok := KnownModels[modelID]
	if !ok {
		return ModelSpec{}, fmt.Errorf("unknown model %q", modelID)
	}
	return spec, nil
}

// ListModels returns all known model specs.
func ListModels() []ModelSpec {
	models := make([]ModelSpec, 0, len(KnownModels))
	for _, spec := range KnownModels {
		models = append(models, spec)
	}
	return models
}
