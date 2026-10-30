package antigravity

import (
	"os/exec"
	"slices"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestResumeCommand(t *testing.T) {
	bin, err := exec.LookPath("agy")
	if err != nil {
		t.Skip("agy not on PATH")
	}
	info, err := (&Store{}).ResumeCommand(thinkt.SessionMeta{ID: "c1", ProjectPath: "/w/ds4dg"})
	if err != nil || info.Command != bin || !slices.Equal(info.Args, []string{"agy", "--conversation", "c1"}) || info.Dir != "/w/ds4dg" {
		t.Fatalf("%+v %v", info, err)
	}
}

func TestResumeCommandWithoutAgy(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := (&Store{}).ResumeCommand(thinkt.SessionMeta{ID: "c1"}); err == nil {
		t.Fatal("no error without agy")
	}
}
