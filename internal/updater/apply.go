package updater

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const applyUpdateArg = "--samuelclient-apply-update"

// StartApply launches a copy of the current executable as a small helper. The
// helper waits for this process to exit before replacing the executable.
func StartApply(stagedPath, executablePath, workingDirectory string, launchArgs []string) error {
	executablePath, err := filepath.EvalSymlinks(executablePath)
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	helper, err := copyHelperExecutable(executablePath)
	if err != nil {
		return err
	}
	args := []string{
		applyUpdateArg,
		strconv.Itoa(os.Getpid()),
		stagedPath,
		executablePath,
		workingDirectory,
		"--",
	}
	args = append(args, launchArgs...)
	cmd := exec.Command(helper, args...)
	configureUpdaterProcess(cmd)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(helper)
		return fmt.Errorf("start update helper: %w", err)
	}
	_ = cmd.Process.Release()
	return nil
}

// IsApplyHelperArgs reports whether this process was started in updater mode.
func IsApplyHelperArgs(args []string) bool {
	return len(args) > 1 && args[1] == applyUpdateArg
}

// RunApplyHelper waits for the app to exit, installs the staged executable,
// and restarts it with the original arguments.
func RunApplyHelper(args []string) error {
	if len(args) < 7 || args[1] != applyUpdateArg {
		return errors.New("invalid updater helper arguments")
	}
	defer os.Remove(args[0])
	pid, err := strconv.Atoi(args[2])
	if err != nil || pid <= 0 {
		return errors.New("invalid updater parent process id")
	}
	stagedPath, targetPath, workingDirectory := args[3], args[4], args[5]
	if args[6] != "--" {
		return errors.New("invalid updater launch arguments")
	}
	if !strings.HasPrefix(strings.ToLower(filepath.Base(targetPath)), "samuelclient") {
		return errors.New("refusing to replace an unexpected executable")
	}
	if !sameDirectory(filepath.Dir(stagedPath), filepath.Dir(targetPath)) ||
		!strings.HasPrefix(filepath.Base(stagedPath), ".SamuelClient-update-") {
		return errors.New("staged executable is outside the application directory")
	}
	if err := waitForProcessExit(pid); err != nil {
		return fmt.Errorf("wait for application to exit: %w", err)
	}
	if err := replaceExecutable(stagedPath, targetPath); err != nil {
		return fmt.Errorf("replace application executable: %w", err)
	}
	cmd := exec.Command(targetPath, args[7:]...)
	cmd.Dir = workingDirectory
	configureRestartProcess(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("restart updated application: %w", err)
	}
	_ = cmd.Process.Release()
	return nil
}

func copyHelperExecutable(source string) (string, error) {
	tempDirectory := os.TempDir()
	pattern := "SamuelClient-update-helper-*"
	if strings.EqualFold(filepath.Ext(source), ".exe") {
		pattern += ".exe"
	}
	file, err := os.CreateTemp(tempDirectory, pattern)
	if err != nil {
		return "", fmt.Errorf("create updater helper: %w", err)
	}
	path := file.Name()
	removeOnFailure := true
	defer func() {
		if removeOnFailure {
			_ = os.Remove(path)
		}
	}()

	input, err := os.Open(source)
	if err != nil {
		_ = file.Close()
		return "", fmt.Errorf("open updater helper source: %w", err)
	}
	if _, err := io.Copy(file, input); err != nil {
		_ = input.Close()
		_ = file.Close()
		return "", fmt.Errorf("copy updater helper: %w", err)
	}
	if err := input.Close(); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("close updater helper source: %w", err)
	}
	if err := file.Chmod(0700); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("set updater helper permissions: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close updater helper: %w", err)
	}
	removeOnFailure = false
	return path, nil
}

func sameDirectory(a, b string) bool {
	a, errA := filepath.Abs(a)
	b, errB := filepath.Abs(b)
	return errA == nil && errB == nil && filepath.Clean(a) == filepath.Clean(b)
}
