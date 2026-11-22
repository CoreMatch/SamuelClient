//go:build !windows

package updater

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

func configureUpdaterProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func configureRestartProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func waitForProcessExit(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	for {
		err = process.Signal(syscall.Signal(0))
		if err != nil {
			if err == os.ErrProcessDone {
				return nil
			}
			if errno, ok := err.(syscall.Errno); ok && errno == syscall.ESRCH {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func replaceExecutable(stagedPath, targetPath string) error {
	return os.Rename(stagedPath, targetPath)
}
