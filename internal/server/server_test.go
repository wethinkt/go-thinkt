package server

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Isolate tests from the real ~/.thinkt directory (and its indexer socket)
	// so that RPC calls like indexerListProjects fall through to the test store.
	tmp, err := os.MkdirTemp("", "thinkt-server-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)
	os.Setenv("THINKT_HOME", tmp)
	os.Exit(m.Run())
}
