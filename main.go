package main

import (
	"embed"
	"fmt"
	"os"

	"HRPMLW/internal/interop"
	"HRPMLW/internal/updater"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// version is set for release builds with -ldflags "-X main.version=vX.Y.Z".
var version = "dev"

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if updater.IsApplyHelperArgs(os.Args) {
		if err := updater.RunApplyHelper(os.Args); err != nil {
			fmt.Fprintln(os.Stderr, "[updater]", err)
			os.Exit(1)
		}
		return
	}

	// Create an instance of the app structure
	app := NewApp()

	// Deep link received as argv of this process (the .desktop
	// x-scheme-handler passes matchcore:// URLs this way, INTEROP-SPEC §10).
	app.HandleDeepLinkArg(interop.FindDeepLinkArg(os.Args))

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "Samuel Client",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		// Single instance: a second launch (e.g. the OS opening a deep link
		// while the launcher already runs) redirects its argv here instead
		// of starting a parallel process (INTEROP-SPEC §7).
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "com.corematch.samuelclient",
			OnSecondInstanceLaunch: func(secondInstanceData options.SecondInstanceData) {
				app.HandleDeepLinkArg(interop.FindDeepLinkArg(secondInstanceData.Args))
				if app.ctx != nil {
					wailsruntime.WindowUnminimise(app.ctx)
					wailsruntime.WindowShow(app.ctx)
				}
			},
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
