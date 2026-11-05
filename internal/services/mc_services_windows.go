//go:build windows

package services

import (
	"os/exec"
	"syscall"
)

// Windows process creation flags. We use DETACHED_PROCESS so the child has no
// console attached to the launcher and CREATE_NEW_PROCESS_GROUP so it is not
// killed when the launcher (Wails) process exits.
const (
	detachedProcess       uint32 = 0x00000008
	createNewProcessGroup uint32 = 0x00000200
)

// configureDetachedProcess configures the SysProcAttr so the launched Java
// process outlives the launcher. Without DETACHED_PROCESS + a new process
// group Windows would terminate the child when the parent (Wails) exits.
func configureDetachedProcess(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= detachedProcess | createNewProcessGroup
	cmd.SysProcAttr.HideWindow = true
}
