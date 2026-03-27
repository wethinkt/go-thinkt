package summarize

import "github.com/wethinkt/go-thinkt/internal/index/llm"

const DefaultModelID = "qwen2.5-3b-instruct"

// ModelSpec describes a known generative model for summarization.
type ModelSpec = llm.ModelSpec

// LookupModel returns the ModelSpec for a given model ID.
func LookupModel(modelID string) (ModelSpec, error) {
	if modelID == "" {
		modelID = DefaultModelID
	}
	return llm.LookupModel(modelID)
}

// ModelPathForID returns ~/.thinkt/models/{filename} for a known model ID.
func ModelPathForID(modelID string) (string, error) {
	if modelID == "" {
		modelID = DefaultModelID
	}
	return llm.ModelPath(modelID)
}

// DefaultModelPath returns the path for the default summarization model.
func DefaultModelPath() (string, error) {
	return ModelPathForID(DefaultModelID)
}

// EnsureModel downloads the specified model if it is not already present.
func EnsureModel(modelID string, onProgress func(downloaded, total int64)) error {
	if modelID == "" {
		modelID = DefaultModelID
	}
	return llm.EnsureModel(modelID, onProgress)
}
