package main

import (
	"embed"
	"os"
	"path/filepath"

	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
	"go.dalw.in/holocron/internal/notes"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	// Register a custom event whose associated data type is string.
	// This is not required, but the binding generator will pick up registered events
	// and provide a strongly typed JS/TS API for them.
	application.RegisterEvent[string]("time")
}

// dataDir falls back so it also works where UserConfigDir is unavailable (mobile sandboxes).
func dataDir() string {
	for _, f := range []func() (string, error){os.UserConfigDir, os.UserHomeDir} {
		if d, err := f(); err == nil && d != "" {
			return filepath.Join(d, "holocron")
		}
	}
	return filepath.Join(os.TempDir(), "holocron")
}

// main function serves as the application's entry point. It initializes the application, creates a window,
// and starts a goroutine that emits a time-based event every second. It subsequently runs the application and
// logs any error that might occur.
func main() {

	svc, err := notes.New(dataDir())
	if err != nil {
		log.Fatal(err)
	}

	// Create a new Wails application by providing the necessary options.
	// Variables 'Name' and 'Description' are for application metadata.
	// 'Assets' configures the asset server with the 'FS' variable pointing to the frontend files.
	// 'Bind' is a list of Go struct instances. The frontend has access to the methods of these instances.
	// 'Mac' options tailor the application when running an macOS.
	app := application.New(application.Options{
		Name:        "Holocron",
		Description: "Markdown notes that sync",
		Services: []application.Service{
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "Holocron", Width: 1100, Height: 720, MinWidth: 360, MinHeight: 480, URL: "/",
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
