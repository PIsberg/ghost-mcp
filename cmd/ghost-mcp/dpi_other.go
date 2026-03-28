//go:build !windows

package main

// getDPIScale returns 1.0 on non-Windows platforms.
// On macOS and Linux the robotgo coordinate space already matches the logical
// pixel space that applications use, so no scaling correction is needed.
func getDPIScale() float64 {
	return 1.0
}
