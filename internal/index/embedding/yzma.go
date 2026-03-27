package embedding

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/wethinkt/go-thinkt/internal/index/llm"
)

const (
	DefaultModelID = "nomic-embed-text-v1.5"
	maxCtxTokens   = 2048
)

// poolingTypeForModel returns the llama pooling type for a given model.
// Nomic models use mean pooling; Qwen models use last-token pooling.
func poolingTypeForModel(spec llm.ModelSpec) llama.PoolingType {
	if spec.Dim == 1024 { // qwen3-embedding-0.6b
		return llama.PoolingTypeLast
	}
	return llama.PoolingTypeMean // nomic and default
}

// Embedder wraps a yzma/llama model for in-process text embedding.
// It is safe for concurrent use.
type Embedder struct {
	model   llama.Model
	ctx     llama.Context
	vocab   llama.Vocab
	dim     int32
	modelID string
	mu      sync.Mutex
}

// NewEmbedder loads a GGUF embedding model and returns a ready-to-use Embedder.
// modelID selects the model from llm.KnownModels (empty = DefaultModelID).
// modelPath overrides the file path (empty = derived from modelID via llm.ModelPath).
// The caller must call Close() when done.
func NewEmbedder(modelID, modelPath string) (*Embedder, error) {
	if modelID == "" {
		modelID = DefaultModelID
	}

	spec, err := llm.LookupModel(modelID)
	if err != nil {
		return nil, err
	}

	if modelPath == "" {
		p, err := llm.ModelPath(spec.ID)
		if err != nil {
			return nil, fmt.Errorf("model path: %w", err)
		}
		modelPath = p
	}

	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("model file not found: %w", err)
	}

	libPath, err := llm.EnsureRuntime()
	if err != nil {
		return nil, fmt.Errorf("ensure runtime: %w", err)
	}

	if err := llama.Load(libPath); err != nil {
		return nil, fmt.Errorf("load llama runtime: %w", err)
	}

	llama.LogSet(llama.LogSilent())
	llama.Init()

	model, err := llama.ModelLoadFromFile(modelPath, llama.ModelDefaultParams())
	if err != nil {
		llama.Close()
		return nil, fmt.Errorf("load model: %w", err)
	}

	ctxParams := llama.ContextDefaultParams()
	ctxParams.NCtx = uint32(maxCtxTokens)
	ctxParams.NBatch = uint32(maxCtxTokens)
	ctxParams.NUbatch = uint32(maxCtxTokens) // physical batch must match logical for embedding
	ctxParams.PoolingType = poolingTypeForModel(spec)
	ctxParams.Embeddings = 1

	ctx, err := llama.InitFromModel(model, ctxParams)
	if err != nil {
		_ = llama.ModelFree(model)
		llama.Close()
		return nil, fmt.Errorf("init context: %w", err)
	}

	nEmbd := llama.ModelNEmbd(model)
	if nEmbd <= 0 {
		nEmbd = llama.ModelNEmbdOut(model)
	}
	if nEmbd <= 0 {
		_ = llama.Free(ctx)
		_ = llama.ModelFree(model)
		llama.Close()
		return nil, errors.New("model reports invalid embedding dimension")
	}

	return &Embedder{
		model:   model,
		ctx:     ctx,
		vocab:   llama.ModelGetVocab(model),
		dim:     nEmbd,
		modelID: spec.ID,
	}, nil
}

// EmbedResult holds the output of an Embed call.
type EmbedResult struct {
	Vectors     [][]float32
	TotalTokens int
}

// Embed produces L2-normalized embedding vectors for the given texts.
// Texts are batched into single decode calls for efficiency.
// It is safe for concurrent use (calls are serialized internally).
func (e *Embedder) Embed(_ context.Context, texts []string) (*EmbedResult, error) {
	if len(texts) == 0 {
		return &EmbedResult{}, nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Tokenize all texts upfront.
	var allTokenized []tokenized
	for i, text := range texts {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		tokens := llama.Tokenize(e.vocab, text, true, true)
		if len(tokens) == 0 {
			continue
		}
		if len(tokens) > maxCtxTokens {
			tokens = tokens[:maxCtxTokens]
		}
		allTokenized = append(allTokenized, tokenized{tokens: tokens, index: i})
	}

	totalTokens := 0
	for _, t := range allTokenized {
		totalTokens += len(t.tokens)
	}

	results := make([][]float32, len(texts))

	// Process in batches that fit within the context window.
	for len(allTokenized) > 0 {
		// Greedily pack texts into a batch until we'd exceed maxCtxTokens.
		var batchItems []tokenized
		totalTokens := 0
		for _, t := range allTokenized {
			if totalTokens+len(t.tokens) > maxCtxTokens && len(batchItems) > 0 {
				break
			}
			batchItems = append(batchItems, t)
			totalTokens += len(t.tokens)
		}
		allTokenized = allTokenized[len(batchItems):]

		vecs, err := e.decodeBatch(batchItems)
		if err != nil {
			return nil, err
		}
		for i, item := range batchItems {
			normalizeL2(vecs[i])
			results[item.index] = vecs[i]
		}
	}

	// Fill in zero vectors for any empty/skipped texts.
	for i := range results {
		if results[i] == nil {
			results[i] = make([]float32, e.dim)
		}
	}

	return &EmbedResult{Vectors: results, TotalTokens: totalTokens}, nil
}

// decodeBatch embeds multiple tokenized texts in a single decode call,
// using distinct sequence IDs to separate them.
func (e *Embedder) decodeBatch(items []tokenized) ([][]float32, error) {
	// Clear memory before batch. Some embedding-only models (e.g. nomic)
	// return a nil memory handle — skip the clear in that case.
	if mem, err := llama.GetMemory(e.ctx); err != nil {
		return nil, fmt.Errorf("get memory: %w", err)
	} else if mem != 0 {
		if err := llama.MemoryClear(mem, true); err != nil {
			return nil, fmt.Errorf("clear memory: %w", err)
		}
	}

	// Process each text sequentially with BatchGetOne + Decode.
	// BatchInit+Add batching produces zero embeddings (likely a yzma/FFI issue),
	// so we use the proven single-sequence approach for each text.
	vecs := make([][]float32, len(items))
	for i, item := range items {
		batch := llama.BatchGetOne(item.tokens)
		if _, err := llama.Decode(e.ctx, batch); err != nil {
			return nil, fmt.Errorf("decode seq %d: %w", i, err)
		}
		if err := llama.Synchronize(e.ctx); err != nil {
			return nil, fmt.Errorf("synchronize seq %d: %w", i, err)
		}

		vec, err := llama.GetEmbeddingsSeq(e.ctx, 0, e.dim)
		if err != nil {
			return nil, fmt.Errorf("get embeddings seq %d: %w", i, err)
		}
		if len(vec) == 0 {
			return nil, fmt.Errorf("empty embedding for seq %d", i)
		}
		vecs[i] = append([]float32(nil), vec...)

		// Clear memory for next text in the batch.
		if i < len(items)-1 {
			if mem, err := llama.GetMemory(e.ctx); err != nil {
				return nil, fmt.Errorf("get memory: %w", err)
			} else if mem != 0 {
				if err := llama.MemoryClear(mem, true); err != nil {
					return nil, fmt.Errorf("clear memory: %w", err)
				}
			}
		}
	}

	return vecs, nil
}

type tokenized struct {
	tokens []llama.Token
	index  int
}

// Dim returns the embedding dimension.
func (e *Embedder) Dim() int { return int(e.dim) }

// EmbedModelID returns the model identifier string.
func (e *Embedder) EmbedModelID() string { return e.modelID }

// Close releases all llama resources.
func (e *Embedder) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()

	_ = llama.Free(e.ctx)
	_ = llama.ModelFree(e.model)
	llama.Close()
}

// --- L2 normalization ---

func normalizeL2(vec []float32) {
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return
	}
	norm := float32(1.0 / math.Sqrt(sum))
	for i := range vec {
		vec[i] *= norm
	}
}
