package db

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func trySnapshotCopy(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer out.Close()

	if err := unix.IoctlFileClone(int(out.Fd()), int(in.Fd())); err != nil {
		os.Remove(dst)
		if errors.Is(err, unix.EXDEV) || errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) {
			return errSnapshotCopyUnavailable
		}
		return err
	}
	return nil
}
