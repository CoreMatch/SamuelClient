package main

import (
	"HRPMLW/internal/database"
	"HRPMLW/internal/interop"
	"HRPMLW/internal/repository"
	"HRPMLW/internal/services"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// App struct
//
// ADR-0004: the frontend reaches the backend exclusively through the Wails
// binding methods in bindings.go. There is no frontend-facing HTTP listener;
// the previous embedded Gin server, its routes/middleware, the HTTP client,
// and the SERVER_PORT configuration have all been retired.
//
// The match handoff listener (INTEROP-SPEC.md) is a separate, inter-process
// boundary: a loopback-only HTTP server authenticated by a per-process token
// in a 0600 discovery file. It never serves the frontend and never carries
// credentials.
type App struct {
	ctx context.Context

	authService      *services.AuthService
	microsoftService *services.MicrosoftService
	bmclService      *services.BMCLAPIService
	launcherService  *services.LauncherService
	javaRepo         *repository.JavaRepository
	tokenStore       services.TokenStore

	// Install cancellation (PENDING-DECISIONS #10): one cancellable context per
	// in-flight download/install, keyed by version id, so CancelInstall can
	// abort it. Guarded by installMu.
	installMu      sync.Mutex
	installCancels map[string]*installCancel

	// Match handoff state (INTEROP-SPEC §6): single pending slot plus the
	// loopback server publishing it.
	interopServer  *interop.Server
	handoffMu      sync.Mutex
	pendingHandoff *interop.HandoffPayload
	started        bool
	deepLink       *interop.DeepLink
}

// installCancel pairs a running install's cancel func with an identity the
// cleanup path can compare, so a finished install never deletes a newer
// install registered under the same version id.
type installCancel struct {
	cancel context.CancelFunc
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		authService:      services.NewAuthService(),
		microsoftService: services.NewMicrosoftService(),
		bmclService:      services.NewBMCLAPIService(),
		launcherService:  services.NewLauncherService(),
		javaRepo:         repository.NewJavaRepository(),
		tokenStore:       services.NewKeychainTokenStore(),
		installCancels:   map[string]*installCancel{},
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Initialize database
	if err := database.Initialize(database.DefaultConfig()); err != nil {
		fmt.Printf("Failed to initialize database: %v\n", err)
	}

	a.startInterop()

	// A deep link received before startup completed (argv of this process)
	// is acted on now that the listener exists.
	a.handoffMu.Lock()
	link := a.deepLink
	a.handoffMu.Unlock()
	if link != nil {
		a.followDeepLink(link)
	}
}

// startInterop brings up the loopback handoff listener and publishes the
// discovery file. Every failure is logged and non-fatal: the launcher works
// without the handoff feature.
func (a *App) startInterop() {
	if err := interop.RegisterDeepLinkHandler(); err != nil {
		fmt.Printf("[interop] deep-link registration: %v\n", err)
	}

	srv, err := interop.NewServer(interop.AppReceiver, a.acceptHandoff, func(format string, args ...interface{}) {
		fmt.Printf("[interop] "+format+"\n", args...)
	})
	if err != nil {
		fmt.Printf("[interop] create listener: %v\n", err)
		return
	}
	if err := srv.Start(); err != nil {
		fmt.Printf("[interop] start listener: %v\n", err)
		return
	}
	a.interopServer = srv

	if _, err := interop.WriteDiscovery(interop.AppReceiver, srv.Port(), srv.Token()); err != nil {
		fmt.Printf("[interop] write discovery file: %v\n", err)
	} else {
		fmt.Printf("[interop] handoff listener on 127.0.0.1:%d\n", srv.Port())
	}

	a.handoffMu.Lock()
	a.started = true
	a.handoffMu.Unlock()
}

// shutdown is called when the app is closing
func (a *App) shutdown(ctx context.Context) {
	if a.interopServer != nil {
		a.interopServer.Stop()
	}

	// Close database connection
	if err := database.Close(); err != nil {
		fmt.Printf("Failed to close database: %v\n", err)
	}
}

// HandleDeepLinkArg processes one matchcore:// URL arriving from argv (this
// process or a second instance redirected through the single-instance lock).
// The deep link is an invocation hint only; the real parameters are pulled
// from the sender over the loopback channel (INTEROP-SPEC §7 flow B).
func (a *App) HandleDeepLinkArg(raw string) {
	if raw == "" {
		return
	}
	link, err := interop.ParseDeepLink(raw)
	if err != nil {
		fmt.Printf("[interop] ignoring invalid deep link: %v\n", err)
		return
	}

	a.handoffMu.Lock()
	if link.Version != interop.ProtocolVersion {
		a.handoffMu.Unlock()
		a.emit("interop:handoff", map[string]interface{}{
			"status":  "version_mismatch",
			"message": fmt.Sprintf("Match handoff protocol version %d is not supported by this SamuelClient (supported: %d). Please update the out-of-date app.", link.Version, interop.ProtocolVersion),
		})
		return
	}
	if a.started {
		a.handoffMu.Unlock()
		a.followDeepLink(link)
		return
	}
	a.deepLink = link
	a.handoffMu.Unlock()
}

// followDeepLink pulls the pending handoff from the sender for up to 30 s
// (the sender may still be publishing its discovery file). Outcomes are
// pushed to the frontend as interop:handoff events; nothing is dropped
// silently.
func (a *App) followDeepLink(link *interop.DeepLink) {
	go func() {
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			doc, _ := interop.LoadLiveSenderDiscovery()
			if doc != nil {
				if doc.ProtocolVersion != interop.ProtocolVersion {
					a.emit("interop:handoff", map[string]interface{}{
						"status":  "version_mismatch",
						"message": fmt.Sprintf("The matchmaking client speaks protocol version %d, this SamuelClient speaks %d. Please update the out-of-date app.", doc.ProtocolVersion, interop.ProtocolVersion),
					})
					return
				}
				payload, err := interop.FetchPendingHandoff(doc, link.HandoffID, 3*time.Second)
				if err == nil {
					if verr := payload.Validate(time.Now()); verr != nil {
						fmt.Printf("[interop] pulled handoff failed validation: %v\n", verr)
						a.emit("interop:handoff", map[string]interface{}{
							"status":  "invalid",
							"message": "The match handoff from the matchmaking client is malformed and was discarded.",
						})
						return
					}
					fmt.Printf("[interop] handoff pulled from sender: %s match=%s target=%s:%d\n",
						payload.HandoffID, payload.Match.MatchID, payload.Match.JoinTarget.Host, payload.Match.JoinTarget.Port)
					if aerr := a.acceptHandoff(payload); aerr != nil {
						// A push arrived first (slot busy) — that copy wins.
						fmt.Printf("[interop] pull skipped: %v\n", aerr)
					}
					return
				}
				if errors.Is(err, interop.ErrHandoffGone) {
					a.emit("interop:handoff", map[string]interface{}{
						"status":  "gone",
						"message": "The matchmaking client no longer holds this match handoff (already joined or expired).",
					})
					return
				}
				fmt.Printf("[interop] pull attempt failed: %v\n", err)
			}
			time.Sleep(2 * time.Second)
		}
		a.emit("interop:handoff", map[string]interface{}{
			"status":     "pull_timeout",
			"handoff_id": link.HandoffID,
			"message":    "Could not fetch the match details from the matchmaking client. Return to it and retry, or keep this launcher open and try again.",
		})
	}()
}

// acceptHandoff is the interop.Server accept hook: it fills the single
// pending slot and notifies the frontend. The payload carries no credentials
// by protocol design, so it is forwarded to the UI as-is.
func (a *App) acceptHandoff(payload *interop.HandoffPayload) error {
	a.handoffMu.Lock()
	if a.pendingHandoff != nil {
		a.handoffMu.Unlock()
		return interop.ErrSlotBusy
	}
	a.pendingHandoff = payload
	a.handoffMu.Unlock()

	a.emit("interop:handoff", map[string]interface{}{
		"status":  "pending",
		"handoff": payload,
	})
	return nil
}
