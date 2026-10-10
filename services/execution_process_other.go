//go:build !aix && !darwin && !dragonfly && !freebsd && !illumos && !linux && !netbsd && !openbsd && !solaris

package services

import (
	"os/exec"
	"time"
)

func configureJobProcess(command *exec.Cmd, _, waitDelay time.Duration) {
	command.WaitDelay = waitDelay
}
