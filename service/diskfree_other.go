//go:build !linux

package service

import "errors"

// availableBytes is only implemented for Linux (the container platform).
func availableBytes(string) (uint64, error) {
	return 0, errors.New("free space reporting is not supported on this platform")
}
