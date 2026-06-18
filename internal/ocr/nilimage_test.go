package ocr

import "testing"

// TestEncodeForOCR_NilImage guards the crash fix: robotgo.CaptureImg returns a
// nil image for an invalid/off-screen region, and encodeForOCR runs inside
// PrepareParallelImageSet's goroutines — a nil dereference there panics on a
// goroutine the caller cannot recover, crashing the whole server. encodeForOCR
// must return an error for a nil (or empty) image instead of dereferencing it.
func TestEncodeForOCR_NilImage(t *testing.T) {
	for _, opts := range []Options{
		{},
		{BrightText: true},
		{DarkText: true},
		{ColorInverted: true},
		{Color: true},
		{Inverted: true},
	} {
		if _, err := encodeForOCR(nil, ScaleFactor, opts); err == nil {
			t.Errorf("encodeForOCR(nil, %+v): expected error, got nil", opts)
		}
	}
}

func TestPrepareParallelImageSet_NilImage(t *testing.T) {
	if _, err := PrepareParallelImageSet(nil, true); err == nil {
		t.Error("PrepareParallelImageSet(nil): expected error, got nil")
	}
	if _, err := PrepareParallelImageSet(nil, false); err == nil {
		t.Error("PrepareParallelImageSet(nil, color): expected error, got nil")
	}
}
