//go:build integration

package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ghost-mcp/mcpclient"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestIntegration_FunctionSweep exercises the tools not covered elsewhere and,
// after every call, verifies the server is still alive via get_screen_size. The
// liveness check is the key assertion: it proves the failsafe-guard fix — a
// find_and_click that computes a (0,0) target must now return a graceful error
// instead of moving the mouse to the origin and shutting the whole server down.
//
// Run with: INTEGRATION=1 GHOST_MCP_TOKEN=test go test -run FunctionSweep
func TestIntegration_FunctionSweep(t *testing.T) {
	skipIfNoGCC(t)
	skipIfNoDisplay(t)
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run the live function sweep")
	}

	_, cleanup := startFixtureServer(t)
	defer cleanup()
	waitForFixture(t) // opens and focuses the fixture (FocusFixtureWindow on Windows)
	time.Sleep(settleTime)

	client, err := mcpclient.NewClient(mcpclient.Config{
		BinaryPath: "../../ghost-mcp.exe", // the live/guarded binary at repo root
		Timeout:    testTimeout,
		Env:        []string{"GHOST_MCP_TOKEN=test", "GHOST_MCP_LEARNING=1"},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()
	ctx := context.Background()

	alive := func(after string) {
		w, h, err := client.GetScreenSize(ctx)
		if err != nil || w <= 0 || h <= 0 {
			t.Fatalf("SERVER NOT ALIVE after %s: get_screen_size err=%v (%dx%d) — the call crashed the server", after, err, w, h)
		}
	}

	call := func(name string, args map[string]interface{}) {
		res, err := client.CallToolString(ctx, name, args)
		if len(res) > 160 {
			res = res[:160]
		}
		t.Logf("%-26s -> err=%v res=%s", name, err, res)
		alive(name) // crash check after every tool
	}

	// Learn once.
	call("set_learning_mode", map[string]interface{}{"enabled": true})
	call("learn_screen", map[string]interface{}{"max_pages": 3})

	// Crash-repro targets: the exact one that killed the server, plus colored
	// buttons and others. None may take the server down.
	for _, tgt := range []string{"Option 1", "Primary", "Choice A", "Click Me", "Success"} {
		call("find_and_click", map[string]interface{}{"text": tgt})
	}

	// Remaining functions in the sweep.
	call("find_and_click_all", map[string]interface{}{"texts": []string{"Option 1", "Option 2"}})
	call("smart_click", map[string]interface{}{"text": "Option 3"})
	call("scroll", map[string]interface{}{"direction": "down", "amount": 3})
	call("scroll_until_text", map[string]interface{}{"text": "Keyboard", "direction": "down", "max_scrolls": 5})
	call("get_annotated_view", map[string]interface{}{"page_index": 0})
	call("click_until_text_appears", map[string]interface{}{"x": 900, "y": 400, "wait_for_text": "Last Action", "max_clicks": 2})
	call("execute_workflow", map[string]interface{}{"steps": []map[string]interface{}{
		{"action": "wait", "delay_ms": 200},
		{"action": "scroll", "amount": 1, "direction": "down"},
	}})

	t.Log("SWEEP COMPLETE: server survived every call")
}

// TestReproCrashDirect calls the handlers in-process so a panic surfaces with a
// full stack trace (in a test, main()'s buffered file logger is not installed,
// so logging + any panic go straight to the test's stderr).
func TestReproCrashDirect(t *testing.T) {
	skipIfNoGCC(t)
	skipIfNoDisplay(t)
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1")
	}
	_, cleanup := startFixtureServer(t)
	defer cleanup()
	waitForFixture(t)
	time.Sleep(settleTime)

	globalLearner.Enable()
	ctx := context.Background()

	lreq := mcp.CallToolRequest{}
	lreq.Params.Name = "learn_screen"
	lreq.Params.Arguments = map[string]interface{}{"max_pages": 3}
	if _, err := handleLearnScreen(ctx, lreq); err != nil {
		t.Fatalf("learn_screen: %v", err)
	}
	t.Logf("HasView=%v", globalLearner.HasView())

	freq := mcp.CallToolRequest{}
	freq.Params.Name = "find_and_click"
	freq.Params.Arguments = map[string]interface{}{"text": "Option 1"}
	res, err := handleFindAndClick(ctx, freq)
	t.Logf("find_and_click returned WITHOUT crashing: err=%v isError=%v", err, res != nil && res.IsError)
}
