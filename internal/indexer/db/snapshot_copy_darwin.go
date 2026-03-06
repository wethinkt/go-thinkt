package db

import (
	"errors"

	"golang.org/x/sys/unix"
)

func trySnapshotCopy(src, dst string) error {
	if err := unix.Clonefile(src, dst, 0); err != nil {
		if errors.Is(err, unix.EXDEV) || errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.ENOSYS) {
			return errSnapshotCopyUnavailable
		}
		return err
	}
	return nil
}
