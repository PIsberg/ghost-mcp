// fixture_server.go - Simple HTTP server for the test fixture
//
// This server hosts the test fixture HTML page so it can be
// accessed and controlled via the MCP UI automation tools.
package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"
)

//go:embed index.html
var content embed.FS

func main() {
	port := os.Getenv("FIXTURE_PORT")
	if port == "" {
		port = "8765"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data, err := content.ReadFile("index.html")
		if err != nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	addr := ":" + port
	log.Printf("👻 Test Fixture Server starting on http://localhost%s", port)
	log.Printf("   Press Ctrl+C to stop")

	// Automatically open the browser to the fixture page
	go func() {
		time.Sleep(500 * time.Millisecond)
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("cmd", "/c", "start", "http://localhost"+port)
		case "darwin":
			cmd = exec.Command("open", "http://localhost"+port)
		default:
			cmd = exec.Command("xdg-open", "http://localhost"+port)
		}
		if err := cmd.Start(); err != nil {
			log.Printf("Failed to open browser automatically: %v", err)
		}
	}()

	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
