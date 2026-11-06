//go:build linux

package interop

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const desktopFileName = "samuelclient-matchcore.desktop"

// desktopTemplate registers the matchcore:// scheme handler
// (INTEROP-SPEC §10, Linux). NoDisplay keeps it out of app menus; %u
// receives the deep link URL as argv.
const desktopTemplate = `[Desktop Entry]
Type=Application
Name=Samuel Client
Comment=MatchCore match handoff handler
NoDisplay=true
Exec=%s %%u
MimeType=x-scheme-handler/matchcore;
Terminal=false
`

// RegisterDeepLinkHandler idempotently installs the .desktop entry and sets
// it as the default handler for x-scheme-handler/matchcore. Best-effort:
// every failure is reported but never fatal (the push flow works without the
// deep link).
func RegisterDeepLinkHandler() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home dir: %w", err)
	}
	appDir := filepath.Join(home, ".local", "share", "applications")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return fmt.Errorf("create applications dir: %w", err)
	}

	content := fmt.Sprintf(desktopTemplate, exe)
	path := filepath.Join(appDir, desktopFileName)

	// Idempotent: skip the rewrite (and the xdg calls) when unchanged.
	if existing, err := os.ReadFile(path); err == nil && string(existing) == content {
		return nil
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write desktop entry: %w", err)
	}

	// Best-effort default-handler registration + MIME cache refresh.
	if xdgMime, err := exec.LookPath("xdg-mime"); err == nil {
		_ = exec.Command(xdgMime, "default", desktopFileName, "x-scheme-handler/matchcore").Run()
	}
	if updateDB, err := exec.LookPath("update-desktop-database"); err == nil {
		_ = exec.Command(updateDB, appDir).Run()
	}
	return nil
}
