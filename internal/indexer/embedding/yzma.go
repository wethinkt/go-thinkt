package embedding

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/hybridgroup/yzma/pkg/download"
	"github.com/hybridgroup/yzma/pkg/llama"
)

const (
	DefaultModelName = "Qwen3-Embedding-0.6B-Q8_0.gguf"
	DefaultModelURL  = "https://huggingface.co/Qwen/Qwen3-Embedding-0.6B-GGUF/resolve/main/Qwen3-Embedding-0.6B-Q8_0.gguf"
	DefaultModelDir  = "models"
	ModelID          = "qwen3-embedding-0.6b"
)

// Embedder wraps a yzma/llama model for in-process text embedding.
// It is safe for concurrent use.
type Embedder struct {
	model llama.Model
	ctx   llama.Context
	vocab llama.Vocab
	dim   int32
	mu    sync.Mutex
}

// NewEmbedder loads the GGUF model at modelPath and returns a ready-to-use
// Embedder. If modelPath is empty, DefaultModelPath() is used.
// The caller must call Close() when done.
func NewEmbedder(modelPath string) (*Embedder, error) {
	if modelPath == "" {
		p, err := DefaultModelPath()
		if err != nil {
			return nil, fmt.Errorf("default model path: %w", err)
		}
		modelPath = p
	}

	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("model file not found: %w", err)
	}

	libPath, err := ensureRuntime()
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
	ctxParams.NCtx = uint32(2048)
	ctxParams.NBatch = uint32(1024)
	ctxParams.PoolingType = llama.PoolingTypeLast
	ctxParams.Embeddings = 1

	ctx, err := llama.InitFromModel(model, ctxParams)
	if err != nil {
		llama.ModelFree(model)
		llama.Close()
		return nil, fmt.Errorf("init context: %w", err)
	}

	nEmbd := llama.ModelNEmbd(model)
	if nEmbd <= 0 {
		nEmbd = llama.ModelNEmbdOut(model)
	}
	if nEmbd <= 0 {
		llama.Free(ctx)
		llama.ModelFree(model)
		llama.Close()
		return nil, errors.New("model reports invalid embedding dimension")
	}

	return &Embedder{
		model: model,
		ctx:   ctx,
		vocab: llama.ModelGetVocab(model),
		dim:   nEmbd,
	}, nil
}

// Embed produces L2-normalized embedding vectors for the given texts.
// It is safe for concurrent use (calls are serialized internally).
func (e *Embedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	results := make([][]float32, len(texts))
	for i, text := range texts {
		vec, err := e.embedOne(strings.TrimSpace(text))
		if err != nil {
			return nil, fmt.Errorf("embed text %d: %w", i, err)
		}
		normalizeL2(vec)
		results[i] = vec
	}
	return results, nil
}

func (e *Embedder) embedOne(text string) ([]float32, error) {
	if text == "" {
		return nil, errors.New("empty text")
	}

	// Clear memory before each embedding.
	mem, err := llama.GetMemory(e.ctx)
	if err != nil {
		return nil, fmt.Errorf("get memory: %w", err)
	}
	if err := llama.MemoryClear(mem, true); err != nil {
		return nil, fmt.Errorf("clear memory: %w", err)
	}

	tokens := llama.Tokenize(e.vocab, text, true, true)
	if len(tokens) == 0 {
		return nil, errors.New("tokenization produced no tokens")
	}

	batch := llama.BatchGetOne(tokens)
	if _, err := llama.Decode(e.ctx, batch); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if err := llama.Synchronize(e.ctx); err != nil {
		return nil, fmt.Errorf("synchronize: %w", err)
	}

	vec, err := llama.GetEmbeddingsSeq(e.ctx, 0, e.dim)
	if err != nil {
		return nil, fmt.Errorf("get embeddings: %w", err)
	}
	if len(vec) == 0 {
		return nil, errors.New("empty embedding vector")
	}

	// Copy — the underlying slice is owned by llama.
	return append([]float32(nil), vec...), nil
}

// Dim returns the embedding dimension.
func (e *Embedder) Dim() int { return int(e.dim) }

// EmbedModelID returns the model identifier string.
func (e *Embedder) EmbedModelID() string { return ModelID }

// Close releases all llama resources.
func (e *Embedder) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()

	llama.Free(e.ctx)
	llama.ModelFree(e.model)
	llama.Close()
}

// DefaultModelPath returns ~/.thinkt/models/Qwen3-Embedding-0.6B-Q8_0.gguf.
func DefaultModelPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".thinkt", DefaultModelDir, DefaultModelName), nil
}

// EnsureModel downloads the default model if it is not already present.
// onProgress is called with bytes downloaded and total size; it may be nil.
func EnsureModel(onProgress func(downloaded, total int64)) error {
	modelPath, err := DefaultModelPath()
	if err != nil {
		return err
	}

	if fi, err := os.Stat(modelPath); err == nil && !fi.IsDir() {
		return nil // already present
	}

	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		return fmt.Errorf("create model dir: %w", err)
	}

	tmpDir, err := os.MkdirTemp(filepath.Dir(modelPath), "model-download-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	tracker := &progressTracker{onProgress: onProgress}
	if err := download.GetModelWithProgress(DefaultModelURL, tmpDir, tracker); err != nil {
		return fmt.Errorf("download model: %w", err)
	}

	modelFile, err := findGGUFFile(tmpDir, DefaultModelName)
	if err != nil {
		return err
	}

	// Remove any stale target before rename.
	if _, err := os.Stat(modelPath); err == nil {
		_ = os.RemoveAll(modelPath)
	}

	return os.Rename(modelFile, modelPath)
}

func findGGUFFile(root, preferredName string) (string, error) {
	var preferred, fallback string

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		base := filepath.Base(path)
		if base == preferredName {
			preferred = path
		} else if strings.HasSuffix(strings.ToLower(base), ".gguf") && fallback == "" {
			fallback = path
		}
		return nil
	})

	if preferred != "" {
		return preferred, nil
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("no GGUF file found under %s", root)
}

// --- runtime helpers ---

func ensureRuntime() (string, error) {
	libPath := defaultLibPath()

	absPath, err := filepath.Abs(libPath)
	if err != nil {
		return "", err
	}

	if download.AlreadyInstalled(absPath) {
		return absPath, nil
	}

	if err := os.MkdirAll(absPath, 0o755); err != nil {
		return "", err
	}

	proc := autoProcessor()

	version, err := download.LlamaLatestVersion()
	if err != nil {
		return "", fmt.Errorf("get latest llama version: %w", err)
	}

	if err := download.GetWithProgress(runtime.GOARCH, runtime.GOOS, proc, version, absPath, &progressTracker{}); err != nil {
		return "", fmt.Errorf("install runtime: %w", err)
	}

	return absPath, nil
}

func autoProcessor() string {
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		return "metal"
	}
	if ok, _ := download.HasCUDA(); ok {
		return "cuda"
	}
	return "cpu"
}

func defaultLibPath() string {
	if v := os.Getenv("YZMA_LIB"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".yzma", "lib")
	}
	return filepath.Join(home, ".yzma", "lib")
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

// --- progress tracker ---

type progressTracker struct {
	onProgress func(downloaded, total int64)
}

func (t *progressTracker) TrackProgress(_ string, currentSize, totalSize int64, stream io.ReadCloser) io.ReadCloser {
	if t.onProgress == nil {
		return stream
	}
	return &progressReader{
		onProgress:  t.onProgress,
		currentSize: currentSize,
		totalSize:   totalSize,
		reader:      stream,
	}
}

type progressReader struct {
	onProgress  func(downloaded, total int64)
	currentSize int64
	totalSize   int64
	reader      io.ReadCloser
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.currentSize += int64(n)
	pr.onProgress(pr.currentSize, pr.totalSize)
	return n, err
}

func (pr *progressReader) Close() error {
	return pr.reader.Close()
}
