//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package services

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func configureJobProcess(command *exec.Cmd, gracePeriod, waitDelay time.Duration) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.WaitDelay = waitDelay
	command.Cancel = func() error {
		if command.Process == nil {
			return os.ErrProcessDone
		}

		processGroupID := command.Process.Pid
		_ = syscall.Kill(-processGroupID, syscall.SIGTERM)
		time.Sleep(gracePeriod)
		if err := syscall.Kill(-processGroupID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("kill job process group %d: %w", processGroupID, err)
		}
		return nil
	}
}
