//go:build !darwin && !linux

package db

func trySnapshotCopy(src, dst string) error {
	return errSnapshotCopyUnavailable
}
