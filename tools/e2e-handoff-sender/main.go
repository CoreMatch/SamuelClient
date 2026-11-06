// Command e2e-handoff-sender drives the REAL MatchCoreArena-Client sender
// implementation (internal/interop, commit 836b720) against a running
// SamuelClient receiver for the INTEROP-SPEC v1 cross-process verification.
//
// Go's internal-package rule forces this program to be compiled inside the
// MatchCoreArena-Client module; SamuelClient keeps this source under tools/
// as the reproducible record (the sibling go.mod placeholder excludes it
// from this module's build). The e2e run copied main.go (only) into a
// throwaway clone of the MCA client (/tmp/mca-sender-e2e) so the MCA
// repository itself was never modified.
//
// Usage (inside the MCA client module):
//
//	go run ./tools/e2e-handoff-sender
//
// The join target is deliberately unreachable (127.0.0.1:9, the discard
// port) so the receiver degrades with "server unreachable" instead of
// launching a real game.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"MatchCoreArena-Client/internal/interop"
)

func main() {
	mgr, err := interop.NewManager(func() interop.GameServer {
		// Local fallback unused: the match info carries the server-provided
		// join target (contract v1.6.0 precedence).
		return interop.GameServer{}
	})
	if err != nil {
		fmt.Println("create sender manager:", err)
		os.Exit(1)
	}
	if err := mgr.Start(); err != nil {
		fmt.Println("start sender:", err)
		os.Exit(1)
	}
	defer mgr.Stop()

	status := mgr.PrepareHandoff(interop.MatchInfo{
		MatchID:    90001,
		MatchType:  "bedwars",
		MapName:    "Aquarium",
		ServerHost: "127.0.0.1", // unreachable test target
		ServerPort: 9,            // discard port: nothing listens here
	})
	printStatus("initial", status)

	deadline := time.Now().Add(70 * time.Second)
	last := status.State
	for time.Now().Before(deadline) {
		time.Sleep(time.Second)
		status = mgr.Status()
		if status.State != last {
			printStatus("transition", status)
			last = status.State
		}
		switch status.State {
		case interop.StateDelivered:
			printStatus("final", status)
			os.Exit(0)
		case interop.StateTimeout, interop.StateNoReceiver, interop.StateVersionMismatch,
			interop.StateMissingConfig, interop.StateError, interop.StateBusy:
			printStatus("final", status)
			os.Exit(2)
		}
	}
	fmt.Println("driver deadline exceeded without a terminal sender state")
	os.Exit(3)
}

func printStatus(phase string, s interop.Status) {
	data, _ := json.Marshal(s)
	fmt.Printf("[%s] %s\n", phase, data)
}
