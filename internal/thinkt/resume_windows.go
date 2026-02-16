//go:build windows

package thinkt

import (
	"os"
	"os/exec"
)

// ExecResume starts the resume command and exits the current process.
// Windows does not support syscall.Exec, so we use os/exec instead.
func ExecResume(info *ResumeInfo) error {
	cmd := exec.Command(info.Command, info.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if info.Dir != "" {
		cmd.Dir = info.Dir
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil // unreachable
}
