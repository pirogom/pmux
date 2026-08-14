package main

import (
	"embed"
	"flag"
	"fmt"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"pmux/pkg/server"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	attachConsole()

	isServerMode := flag.Bool("server", false, "Run in background daemon server mode")
	serverPort := flag.Int("port", server.GetServerPort(), "Server listen port")
	isStatusMode := flag.Bool("status", false, "Check if background daemon server is running")
	isStopMode := flag.Bool("stop", false, "Stop running background daemon server")
	isKillMode := flag.Bool("kill", false, "Stop running background daemon server (alias for --stop)")
	isHelpMode := flag.Bool("help", false, "Display help message")

	flag.Usage = func() {
		fmt.Println("pmux - Terminal Multiplexer for Windows (tmux alternative)")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Println("  pmux [options]")
		fmt.Println()
		fmt.Println("Options:")
		fmt.Println("  --status         Check if background daemon server is running")
		fmt.Println("  --stop, --kill   Stop running background daemon server")
		fmt.Println("  --server         Run in background daemon server mode")
		fmt.Println("  --port <number>  Server listen port (default: 4799)")
		fmt.Println("  --help, -h       Display this help message")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  pmux             Launch GUI client (starts background daemon if needed)")
		fmt.Println("  pmux --status    Check daemon server status")
		fmt.Println("  pmux --stop      Stop running daemon server")
		fmt.Println("  pmux --server    Start daemon server manually")
	}

	flag.Parse()

	if *isHelpMode {
		flag.Usage()
		return
	}

	port := *serverPort

	if *isStatusMode {
		if server.IsServerRunning(port) {
			fmt.Printf("pmux server is running on port %d.\n", port)
		} else {
			fmt.Printf("pmux server is NOT running on port %d.\n", port)
		}
		return
	}

	if *isStopMode || *isKillMode {
		if !server.IsServerRunning(port) {
			fmt.Printf("pmux server is NOT running on port %d.\n", port)
			return
		}
		app := NewApp()
		if err := app.KillServer(); err != nil {
			fmt.Printf("Failed to stop pmux server: %v\n", err)
		} else {
			fmt.Printf("pmux server running on port %d stopped successfully.\n", port)
		}
		return
	}

	if *isServerMode {
		srv := server.NewServer(port)
		if err := srv.Start(); err != nil {
			fmt.Printf("Server exit with error: %v\n", err)
		}
		return
	}

	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "pmux - pirogom's terminal multiplexer",
		Width:  1280,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 24, G: 24, B: 24, A: 255},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
