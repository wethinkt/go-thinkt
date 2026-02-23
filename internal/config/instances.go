package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// StartBackground starts a command in the background, detached from the current terminal.
func StartBackground(c *exec.Cmd) error {
	applyPlatformBackgroundFlags(c)
	return c.Start()
}

// InstanceType identifies the kind of thinkt process.
type InstanceType string

const (
	InstanceServer       InstanceType = "server"
	InstanceServerMCP    InstanceType = "server-mcp"
	InstanceIndexerServer InstanceType = "indexer-server"
)

// Instance represents a running thinkt process.
type Instance struct {
	Type      InstanceType `json:"type"`
	PID       int          `json:"pid"`
	Port      int          `json:"port,omitempty"`
	Host      string       `json:"host,omitempty"`
	LogPath     string       `json:"log_path,omitempty"`
	HTTPLogPath string       `json:"http_log_path,omitempty"`
	Token       string       `json:"token,omitempty"`
	IndexerPID  int          `json:"indexer_pid,omitempty"`
	StartedAt time.Time    `json:"started_at"`
}

// instancesPath returns the path to the instances file.
func instancesPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "instances.json"), nil
}

// RegisterInstance adds a new instance entry, cleaning stale entries first.
func RegisterInstance(inst Instance) error {
	path, err := instancesPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	instances, _ := readInstances(path)
	instances = cleanStale(instances)
	instances = append(instances, inst)

	return writeInstances(path, instances)
}

// UnregisterInstance removes an instance by PID.
func UnregisterInstance(pid int) error {
	path, err := instancesPath()
	if err != nil {
		return err
	}

	instances, _ := readInstances(path)
	filtered := make([]Instance, 0, len(instances))
	for _, inst := range instances {
		if inst.PID != pid {
			filtered = append(filtered, inst)
		}
	}

	return writeInstances(path, filtered)
}

// ListInstances returns all live instances, cleaning stale entries.
func ListInstances() ([]Instance, error) {
	path, err := instancesPath()
	if err != nil {
		return nil, err
	}

	instances, err := readInstances(path)
	if err != nil {
		return nil, err
	}

	live := cleanStale(instances)
	// Write back cleaned list if we removed any stale entries
	if len(live) != len(instances) {
		_ = writeInstances(path, live) // Ignore error, stale cleanup is best-effort
	}

	return live, nil
}

// FindInstanceByPort returns the instance using the given port, or nil.
func FindInstanceByPort(port int) *Instance {
	instances, err := ListInstances()
	if err != nil {
		return nil
	}
	for _, inst := range instances {
		if inst.Port == port {
			return &inst
		}
	}
	return nil
}

// FindInstanceByType returns the first instance of the given type, or nil.
func FindInstanceByType(t InstanceType) *Instance {
	instances, err := ListInstances()
	if err != nil {
		return nil
	}
	for _, inst := range instances {
		if inst.Type == t {
			return &inst
		}
	}
	return nil
}

// IsProcessAlive checks whether a process with the given PID exists.
func IsProcessAlive(pid int) bool {
	return isProcessAlive(pid)
}

// StopInstance stops a running instance and unregisters it.
func StopInstance(inst Instance) error {
	proc, err := os.FindProcess(inst.PID)
	if err != nil {
		return UnregisterInstance(inst.PID)
	}

	// Try graceful shutdown (SIGTERM)
	if err := stopProcess(proc); err != nil {
		// Fallback to Kill if stopProcess fails or isn't enough
		_ = proc.Kill()
	}

	// Wait for it to exit (best effort)
	for i := 0; i < 10; i++ {
		if !IsProcessAlive(inst.PID) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	return UnregisterInstance(inst.PID)
}

func readInstances(path string) ([]Instance, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var instances []Instance
	if err := json.Unmarshal(data, &instances); err != nil {
		return nil, err
	}
	return instances, nil
}

func writeInstances(path string, instances []Instance) error {
	data, err := json.MarshalIndent(instances, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// cleanStale removes entries whose PID is no longer running.
func cleanStale(instances []Instance) []Instance {
	live := make([]Instance, 0, len(instances))
	for _, inst := range instances {
		if isProcessAlive(inst.PID) {
			live = append(live, inst)
		}
	}
	return live
}
