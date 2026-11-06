//go:build !linux

package interop

import "fmt"

// RegisterDeepLinkHandler is a documented gap off Linux (INTEROP-SPEC §10):
//   - Windows: register HKCU\Software\Classes\matchcore\shell\open\command
//     with the executable path and a "%1" argument.
//   - macOS: declare CFBundleURLTypes (matchcore) in the app bundle and/or
//     call LSSetDefaultHandlerForURLScheme.
//
// The push flow (discovery file + loopback POST) works without deep-link
// registration; only the cold-start wake-up (flow B) is unavailable until
// this is implemented.
func RegisterDeepLinkHandler() error {
	return fmt.Errorf("deep-link registration is not implemented on this platform (Linux-first; see INTEROP-SPEC §10)")
}
