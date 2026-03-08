package summarize

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
)

const maxGenerateTokens = 512

// Summarizer wraps a yzma/llama generative model for text summarization.
// It is safe for concurrent use.
type Summarizer struct {
	model   llama.Model
	ctx     llama.Context
	vocab   llama.Vocab
	sampler llama.Sampler
	nCtx    uint32
	modelID string
	mu      sync.Mutex
}

// NewSummarizer loads a generative GGUF model and returns a ready-to-use Summarizer.
func NewSummarizer(modelID, modelPath string) (*Summarizer, error) {
	spec, err := LookupModel(modelID)
	if err != nil {
		return nil, err
	}

	if modelPath == "" {
		p, err := ModelPathForID(spec.ID)
		if err != nil {
			return nil, fmt.Errorf("model path: %w", err)
		}
		modelPath = p
	}

	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("model file not found: %w", err)
	}

	libPath, err := embedding.EnsureRuntime()
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
	ctxParams.NCtx = spec.NCtx
	ctxParams.NBatch = 512
	ctxParams.NUbatch = 512
	// Embeddings stays 0 (default) for generative mode

	ctx, err := llama.InitFromModel(model, ctxParams)
	if err != nil {
		_ = llama.ModelFree(model)
		llama.Close()
		return nil, fmt.Errorf("init context: %w", err)
	}

	// Greedy sampler for deterministic output.
	samplerParams := llama.DefaultSamplerParams()
	samplerParams.Temp = 0.0
	sampler := llama.NewSampler(model, []llama.SamplerType{llama.SamplerTypeTemperature}, samplerParams)
	if sampler == 0 {
		_ = llama.Free(ctx)
		_ = llama.ModelFree(model)
		llama.Close()
		return nil, errors.New("failed to create sampler")
	}

	return &Summarizer{
		model:   model,
		ctx:     ctx,
		vocab:   llama.ModelGetVocab(model),
		sampler: sampler,
		nCtx:    spec.NCtx,
		modelID: spec.ID,
	}, nil
}

// ModelID returns the model identifier string.
func (s *Summarizer) ModelID() string { return s.modelID }

// Close releases all llama resources.
func (s *Summarizer) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	llama.SamplerFree(s.sampler)
	_ = llama.Free(s.ctx)
	_ = llama.ModelFree(s.model)
	llama.Close()
}

// Generate runs text generation with the given prompt and returns the output.
func (s *Summarizer) Generate(_ context.Context, prompt string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear KV cache.
	if mem, err := llama.GetMemory(s.ctx); err == nil && mem != 0 {
		if err := llama.MemoryClear(mem, true); err != nil {
			return "", fmt.Errorf("clear memory: %w", err)
		}
	}

	// Tokenize prompt.
	tokens := llama.Tokenize(s.vocab, prompt, true, true)
	if len(tokens) == 0 {
		return "", errors.New("empty prompt after tokenization")
	}
	if len(tokens) >= int(s.nCtx)-maxGenerateTokens {
		tokens = tokens[:int(s.nCtx)-maxGenerateTokens]
	}

	// Decode prompt in chunks that fit within NBatch (512).
	const batchSize = 512
	for i := 0; i < len(tokens); i += batchSize {
		end := i + batchSize
		if end > len(tokens) {
			end = len(tokens)
		}
		batch := llama.BatchGetOne(tokens[i:end])
		if _, err := llama.Decode(s.ctx, batch); err != nil {
			return "", fmt.Errorf("decode prompt chunk %d: %w", i/batchSize, err)
		}
	}

	// Autoregressive generation loop.
	var output []llama.Token
	for i := range maxGenerateTokens {
		token := llama.SamplerSample(s.sampler, s.ctx, -1)
		llama.SamplerAccept(s.sampler, token)

		if llama.VocabIsEOG(s.vocab, token) {
			break
		}

		output = append(output, token)

		// Feed the token back.
		tokenBatch := llama.BatchGetOne([]llama.Token{token})
		if _, err := llama.Decode(s.ctx, tokenBatch); err != nil {
			return "", fmt.Errorf("decode token %d: %w", i, err)
		}
	}

	text := llama.Detokenize(s.vocab, output, false, true)
	return strings.TrimSpace(text), nil
}

// Summarize produces a summary and classification of a thinking block.
func (s *Summarizer) Summarize(ctx context.Context, thinkingText string) (*SummaryResult, error) {
	prompt := buildClassifyPrompt(thinkingText)
	raw, err := s.Generate(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return parseClassifyResponse(raw)
}

// SummarizeSession produces a session-level summary from session context text.
func (s *Summarizer) SummarizeSession(ctx context.Context, sessionContext string) (*SessionSummaryResult, error) {
	prompt := buildSessionPrompt(sessionContext)
	raw, err := s.Generate(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return &SessionSummaryResult{Summary: raw}, nil
}
