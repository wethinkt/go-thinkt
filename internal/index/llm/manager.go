package llm

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/hybridgroup/yzma/pkg/download"
	"github.com/wethinkt/go-thinkt/internal/config"
)

const modelDir = "models"

// ModelPath returns the filesystem path for a known model ID.
func ModelPath(modelID string) (string, error) {
	spec, err := LookupModel(modelID)
	if err != nil {
		return "", err
	}
	configDir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, modelDir, spec.FileName), nil
}

// IsModelDownloaded returns true if the model file exists at the given path.
func IsModelDownloaded(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// EnsureModel downloads the specified model if it is not already present.
func EnsureModel(modelID string, onProgress func(downloaded, total int64)) error {
	spec, err := LookupModel(modelID)
	if err != nil {
		return err
	}

	modelPath, err := ModelPath(spec.ID)
	if err != nil {
		return err
	}

	if IsModelDownloaded(modelPath) {
		return nil
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
	if err := download.GetModelWithProgress(spec.URL, tmpDir, tracker); err != nil {
		return fmt.Errorf("download model: %w", err)
	}

	modelFile, err := findGGUFFile(tmpDir, spec.FileName)
	if err != nil {
		return err
	}

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
