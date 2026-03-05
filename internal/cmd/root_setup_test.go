package cmd

import (
	"errors"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestRootPersistentPreRunE_StopsWhenSetupDoesNotCreateConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("THINKT_HOME", tmpDir)
	t.Setenv("THINKT_LOG_FILE", "")
	t.Setenv("THINKT_PROFILE", "")

	prevSetupOK := setupOK
	prevLogPath := logPath
	prevInteractive := runSetupInteractiveFn
	t.Cleanup(func() {
		setupOK = prevSetupOK
		logPath = prevLogPath
		runSetupInteractiveFn = prevInteractive
	})

	setupOK = false
	logPath = ""
	runSetupInteractiveFn = func([]thinkt.StoreFactory) error {
		return nil
	}

	err := rootCmd.PersistentPreRunE(rootCmd, nil)
	if !errors.Is(err, errSetupIncomplete) {
		t.Fatalf("PersistentPreRunE error = %v, want %v", err, errSetupIncomplete)
	}
}

func TestRunSetup_CancelledReturnsNil(t *testing.T) {
	prevInteractive := runSetupInteractiveFn
	t.Cleanup(func() {
		runSetupInteractiveFn = prevInteractive
	})

	runSetupInteractiveFn = func([]thinkt.StoreFactory) error {
		return nil
	}

	if err := runSetup(setupCmd, nil); err != nil {
		t.Fatalf("runSetup returned error for cancelled setup: %v", err)
	}
}
