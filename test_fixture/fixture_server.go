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

	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
