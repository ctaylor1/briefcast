//go:build linux

package service

import "syscall"

// availableBytes reports bytes available to unprivileged users on path's filesystem.
func availableBytes(path string) (uint64, error) {
	var s syscall.Statfs_t
	if err := syscall.Statfs(path, &s); err != nil {
		return 0, err
	}
	return s.Bavail * uint64(s.Bsize), nil
}
