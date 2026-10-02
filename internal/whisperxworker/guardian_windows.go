package whisperxworker

import (
	"errors"
	"os"
	"os/exec"
)

func configureGuardian(*exec.Cmd, *os.File) {}
func RunGuardian([]string) error            { return errors.New("worker process guardian requires Linux") }
