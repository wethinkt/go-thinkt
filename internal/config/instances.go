package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// InstanceType identifies the kind of thinkt process.
type InstanceType string

const (
	InstanceServe        InstanceType = "serve"
	InstanceServeLite    InstanceType = "serve-lite"
	InstanceServeMCP     InstanceType = "serve-mcp"
	InstanceIndexerWatch InstanceType = "indexer-watch"
)

// Instance represents a running thinkt process.
type Instance struct {
	Type      InstanceType `json:"type"`
	PID       int          `json:"pid"`
	Port      int          `json:"port,omitempty"`
	Host      string       `json:"host,omitempty"`
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
		writeInstances(path, live)
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

