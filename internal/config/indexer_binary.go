package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const indexerBinaryName = "thinkt-indexer"

// FindIndexerBinary attempts to locate the thinkt-indexer binary.
// It checks the current executable's directory first, then falls back to PATH.
func FindIndexerBinary() string {
	return findBinaryNearExecutable(indexerBinaryName, runtime.GOOS, os.Executable, os.Stat, exec.LookPath)
}

type executablePathFn func() (string, error)
type statFn func(string) (os.FileInfo, error)
type lookPathFn func(string) (string, error)

func findBinaryNearExecutable(name, goos string, executable executablePathFn, stat statFn, lookPath lookPathFn) string {
	if execPath, err := executable(); err == nil {
		binDir := filepath.Dir(execPath)
		for _, candidateName := range binaryCandidateNames(name, goos) {
			candidate := filepath.Join(binDir, candidateName)
			if _, err := stat(candidate); err == nil {
				return candidate
			}
		}
	}

	for _, candidateName := range binaryCandidateNames(name, goos) {
		if path, err := lookPath(candidateName); err == nil {
			return path
		}
	}

	return ""
}

func binaryCandidateNames(name, goos string) []string {
	candidates := []string{name}
	if goos == "windows" && filepath.Ext(name) == "" {
		candidates = append(candidates, name+".exe")
	}
	return candidates
}
