//go:build !windows

package thinkt

import "syscall"

// ExecResume replaces the current process with the resume command.
// On success, this function does not return.
func ExecResume(info *ResumeInfo) error {
	if info.Dir != "" {
		if err := syscall.Chdir(info.Dir); err != nil {
			return err
		}
	}
	return syscall.Exec(info.Command, info.Args, syscall.Environ())
}
