package whisperxworker

import (
	"golang.org/x/sys/windows"
	"os"
)

func lockState(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
	if err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// Windows hosts exercise the journal/API tests. Production execution is Linux;
// directory fsync and the Linux process guardian are verified in that runtime.
func syncDirectory(string) error { return nil }
func freeBytes(path string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var available uint64
	err = windows.GetDiskFreeSpaceEx(p, &available, nil, nil)
	return available, err
}
