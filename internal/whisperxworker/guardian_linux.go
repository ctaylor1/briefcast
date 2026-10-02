package whisperxworker

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func configureGuardian(cmd *exec.Cmd, owner *os.File) {
	cmd.ExtraFiles = []*os.File{owner}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// RunGuardian is a private re-exec entry point. It holds the inherited state
// ownership lease until the Python process group is stopped and reaped. Losing
// the worker closes stdin, including when the worker receives SIGKILL.
func RunGuardian(args []string) error {
	if len(args) != 3 {
		return errors.New("invalid guardian arguments")
	}
	owner := os.NewFile(3, "state-owner")
	defer owner.Close()
	if _, err := owner.Stat(); err != nil {
		return err
	}
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		return err
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signals)
	gone := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, os.Stdin); close(gone) }()
	// Do not launch inference if the parent was lost during guardian startup.
	select {
	case <-gone:
		return errors.New("worker exited")
	default:
	}
	cmd := exec.Command(args[0], args[1], args[2])
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	if err := cmd.Start(); err != nil {
		return err
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	var err error
	select {
	case err = <-wait:
	case <-gone:
		err = errors.New("worker exited")
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-wait
	case <-signals:
		err = errors.New("worker signalled")
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-wait
	}
	// Reap any ffmpeg/model descendants, even if Python exited successfully while
	// leaving a child behind. No new owner can start while this lease is held.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	deadline := time.Now().Add(5 * time.Second)
	for {
		var status syscall.WaitStatus
		pid, e := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
		if e == syscall.ECHILD {
			break
		}
		if e != nil {
			return errors.Join(err, e)
		}
		if pid == 0 {
			if time.Now().After(deadline) {
				return errors.New("descendant reaping deadline exceeded")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	return err
}
