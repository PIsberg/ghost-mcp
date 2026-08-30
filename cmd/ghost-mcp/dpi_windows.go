//go:build windows

package main

import (
	"os"
	"syscall"

	"github.com/ghost-mcp/internal/logging"
)

// setupDPIAwareness opts the process into per-monitor-v2 DPI awareness when
// GHOST_MCP_DPI_AWARE=1 is set (issue #159). On DPI-scaled Windows desktops
// the default (unaware or system-aware) process sees a virtualised desktop:
// get_screen_size and take_screenshot report logical pixels while parts of
// the physical desktop fall outside the capturable region, so maximized
// windows extend past every capture.
//
// Opt-in rather than default because it changes the coordinate space of every
// capture and click on scaled displays, and must be verified against a live
// desktop per setup. The call must happen before any capture; once another
// component (or a manifest) has set the process DPI awareness the call fails,
// which is logged and otherwise harmless.
func setupDPIAwareness() {
	if os.Getenv("GHOST_MCP_DPI_AWARE") != "1" {
		return
	}

	user32 := syscall.NewLazyDLL("user32.dll")
	proc := user32.NewProc("SetProcessDpiAwarenessContext")
	if err := proc.Find(); err != nil {
		logging.Error("DPI awareness requested but SetProcessDpiAwarenessContext is unavailable (Windows 10 1703+ required): %v", err)
		return
	}

	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 == (DPI_AWARENESS_CONTEXT)-4
	const perMonitorAwareV2 = ^uintptr(3) // two's-complement -4
	ret, _, callErr := proc.Call(perMonitorAwareV2)
	if ret == 0 {
		logging.Error("SetProcessDpiAwarenessContext(PER_MONITOR_AWARE_V2) failed (already set by another component?): %v", callErr)
		return
	}
	logging.Info("Process DPI awareness set to per-monitor-v2 (GHOST_MCP_DPI_AWARE=1)")
}

// getDPIScale returns the system DPI scale factor for the primary monitor.
// 96 DPI = 1.0 (100%), 144 DPI = 1.5 (150%), 192 DPI = 2.0 (200%), etc.
// Returns 1.0 if the DPI cannot be determined.
func getDPIScale() float64 {
	user32 := syscall.NewLazyDLL("user32.dll")
	getDpiForSystem := user32.NewProc("GetDpiForSystem")
	dpi, _, _ := getDpiForSystem.Call()
	if dpi == 0 {
		return 1.0
	}
	return float64(dpi) / 96.0
}
