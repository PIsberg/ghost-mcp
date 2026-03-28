//go:build windows

package main

import "syscall"

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
