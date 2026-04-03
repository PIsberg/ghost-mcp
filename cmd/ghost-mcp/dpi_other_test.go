//go:build !windows

package main

import (
	"testing"
)

func TestGetDPIScaleNonWindows(t *testing.T) {
	scale := getDPIScale()
	if scale != 1.0 {
		t.Errorf("expected DPI scale 1.0 on non-Windows platform, got %f", scale)
	}
}

func TestGetDPIScaleReturnValue(t *testing.T) {
	// On non-Windows platforms, should always return 1.0
	scale := getDPIScale()

	// Verify it's exactly 1.0
	if scale != 1.0 {
		t.Errorf("expected 1.0, got %f", scale)
	}

	// Verify it's a positive number
	if scale <= 0 {
		t.Errorf("expected positive DPI scale, got %f", scale)
	}
}
