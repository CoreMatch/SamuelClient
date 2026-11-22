//go:build windows

package updater

import (
	"os/exec"
	"time"

	"golang.org/x/sys/windows"
)

func configureUpdaterProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

func configureRestartProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

func waitForProcessExit(pid int) error {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		if err == windows.ERROR_INVALID_PARAMETER {
			return nil
		}
		return err
	}
	defer windows.CloseHandle(handle)
	_, err = windows.WaitForSingleObject(handle, windows.INFINITE)
	return err
}

func replaceExecutable(stagedPath, targetPath string) error {
	for attempts := 0; attempts < 50; attempts++ {
		err := windows.MoveFileEx(
			windows.StringToUTF16Ptr(stagedPath),
			windows.StringToUTF16Ptr(targetPath),
			windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
		)
		if err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return windows.MoveFileEx(
		windows.StringToUTF16Ptr(stagedPath),
		windows.StringToUTF16Ptr(targetPath),
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
	)
}
