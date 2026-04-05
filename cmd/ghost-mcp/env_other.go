//go:build !windows

package main

// SetupWindowsEnv is a no-op on non-Windows systems.
func SetupWindowsEnv() {}
