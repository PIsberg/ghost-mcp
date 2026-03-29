// Package cursor provides visual feedback for mouse actions.
package cursor

import (
	"image/color"
	"math"
	"time"

	"github.com/go-vgo/robotgo"
)

// DrawCircle draws a visual circle at the cursor position for feedback.
func DrawCircle(x, y int, radius int, duration time.Duration) {
	// Save current mouse position
	startX, startY := robotgo.GetMousePos()

	// Move to center
	robotgo.Move(x, y)

	// Draw circle by moving mouse in a circle pattern
	steps := 36
	angleStep := 360.0 / float64(steps)

	for i := 0; i <= steps; i++ {
		angle := float64(i) * angleStep
		// Convert to radians
		rad := angle * 3.14159 / 180.0

		// Calculate offset from center
		offsetX := int(float64(radius) * math.Cos(rad))
		offsetY := int(float64(radius) * math.Sin(rad))

		robotgo.Move(x+offsetX, y+offsetY)
		time.Sleep(duration / time.Duration(steps))
	}

	// Return to original position
	robotgo.Move(startX, startY)
}

// FlashCursor briefly highlights the cursor position.
func FlashCursor(x, y int, radius int, color color.RGBA, duration time.Duration) {
	// Note: Full screen overlay requires additional dependencies
	// For now, we'll just move the cursor in a circle pattern
	DrawCircle(x, y, radius, duration)
}

// PulseCursor pulses the cursor position to show a click happened.
func PulseCursor(x, y int) {
	// Quick pulse animation - expand and contract
	for i := 0; i < 2; i++ {
		DrawCircle(x, y, 15, 100*time.Millisecond)
		DrawCircle(x, y, 5, 50*time.Millisecond)
	}
}
