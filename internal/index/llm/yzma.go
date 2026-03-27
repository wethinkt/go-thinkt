package llm

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/hybridgroup/yzma/pkg/download"
)

// GPUMutex serializes access to the local model runtime.
type GPUMutex struct {
	mu sync.Mutex
}

func (g *GPUMutex) Lock()   { g.mu.Lock() }
func (g *GPUMutex) Unlock() { g.mu.Unlock() }

// EnsureRuntime ensures the llama.cpp runtime library is installed and returns its path.
func EnsureRuntime() (string, error) {
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
	proc := AutoProcessor()
	version, err := download.LlamaLatestVersion()
	if err != nil {
		return "", fmt.Errorf("get latest llama version: %w", err)
	}
	if err := download.GetWithProgress(runtime.GOARCH, runtime.GOOS, proc, version, absPath, &progressTracker{}); err != nil {
		return "", fmt.Errorf("install runtime: %w", err)
	}
	return absPath, nil
}

// AutoProcessor detects the best processor type for the current platform.
func AutoProcessor() string {
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
