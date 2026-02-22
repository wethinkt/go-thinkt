package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRegisterAndListInstances(t *testing.T) {
	// Use a temp dir as the config dir
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	inst := Instance{
		Type:      InstanceServer,
		PID:       os.Getpid(),
		Port:      8784,
		Host:      "localhost",
		StartedAt: time.Now(),
	}

	if err := RegisterInstance(inst); err != nil {
		t.Fatalf("RegisterInstance failed: %v", err)
	}

	instances, err := ListInstances()
	if err != nil {
		t.Fatalf("ListInstances failed: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("Expected 1 instance, got %d", len(instances))
	}
	if instances[0].Type != InstanceServer {
		t.Fatalf("Expected type %q, got %q", InstanceServer, instances[0].Type)
	}
	if instances[0].Port != 8784 {
		t.Fatalf("Expected port 8784, got %d", instances[0].Port)
	}
}

func TestUnregisterInstance(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	inst := Instance{
		Type:      InstanceServer,
		PID:       os.Getpid(),
		Port:      8784,
		Host:      "localhost",
		StartedAt: time.Now(),
	}

	if err := RegisterInstance(inst); err != nil {
		t.Fatalf("RegisterInstance failed: %v", err)
	}

	if err := UnregisterInstance(os.Getpid()); err != nil {
		t.Fatalf("UnregisterInstance failed: %v", err)
	}

	instances, err := ListInstances()
	if err != nil {
		t.Fatalf("ListInstances failed: %v", err)
	}
	if len(instances) != 0 {
		t.Fatalf("Expected 0 instances after unregister, got %d", len(instances))
	}
}

func TestStalePIDCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Register an instance with a PID that doesn't exist
	inst := Instance{
		Type:      InstanceIndexerWatch,
		PID:       999999999, // almost certainly not a real PID
		StartedAt: time.Now(),
	}

	if err := RegisterInstance(inst); err != nil {
		t.Fatalf("RegisterInstance failed: %v", err)
	}

	// ListInstances should clean the stale entry
	instances, err := ListInstances()
	if err != nil {
		t.Fatalf("ListInstances failed: %v", err)
	}
	if len(instances) != 0 {
		t.Fatalf("Expected 0 instances after stale cleanup, got %d", len(instances))
	}
}

func TestFindInstanceByPort(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	inst := Instance{
		Type:      InstanceServer,
		PID:       os.Getpid(),
		Port:      8784,
		Host:      "localhost",
		StartedAt: time.Now(),
	}

	if err := RegisterInstance(inst); err != nil {
		t.Fatalf("RegisterInstance failed: %v", err)
	}

	found := FindInstanceByPort(8784)
	if found == nil {
		t.Fatal("Expected to find instance on port 8784")
	}
	if found.PID != os.Getpid() {
		t.Fatalf("Expected PID %d, got %d", os.Getpid(), found.PID)
	}

	notFound := FindInstanceByPort(9999)
	if notFound != nil {
		t.Fatal("Expected nil for unused port")
	}
}

func TestMultipleInstances(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Register two instances with the same PID but different types/ports
	inst1 := Instance{
		Type:      InstanceServer,
		PID:       os.Getpid(),
		Port:      8784,
		Host:      "localhost",
		StartedAt: time.Now(),
	}
	inst2 := Instance{
		Type:      InstanceServerMCP,
		PID:       os.Getpid(),
		Port:      8786,
		Host:      "localhost",
		StartedAt: time.Now(),
	}

	if err := RegisterInstance(inst1); err != nil {
		t.Fatalf("RegisterInstance failed: %v", err)
	}
	if err := RegisterInstance(inst2); err != nil {
		t.Fatalf("RegisterInstance failed: %v", err)
	}

	instances, err := ListInstances()
	if err != nil {
		t.Fatalf("ListInstances failed: %v", err)
	}
	if len(instances) != 2 {
		t.Fatalf("Expected 2 instances, got %d", len(instances))
	}
}

func TestInstancesFileCreation(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	inst := Instance{
		Type:      InstanceServer,
		PID:       os.Getpid(),
		Port:      8784,
		StartedAt: time.Now(),
	}

	if err := RegisterInstance(inst); err != nil {
		t.Fatalf("RegisterInstance failed: %v", err)
	}

	// Check the file was created
	path := filepath.Join(tmpDir, ".thinkt", "instances.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("instances.json was not created at %s", path)
	}
}
