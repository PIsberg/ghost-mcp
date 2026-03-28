// Package visual provides on-screen visual feedback for AI actions.
package visual

import (
	"math"
	"time"

	"github.com/go-vgo/robotgo"
)

// ShowClickEffect draws a visual circle at the click location.
func ShowClickEffect(x, y int) {
	// Store original mouse position
	origX, origY := robotgo.GetMousePos()

	// Quick circle animation around click point
	radius := 20
	steps := 20
	delay := 15 * time.Millisecond

	for i := 0; i <= steps; i++ {
		angle := float64(i) * 360.0 / float64(steps)
		rad := angle * 3.14159 / 180.0

		offsetX := int(float64(radius) * math.Cos(rad))
		offsetY := int(float64(radius) * math.Sin(rad))

		robotgo.Move(x+offsetX, y+offsetY)
		time.Sleep(delay)
	}

	// Return to original position
	robotgo.Move(origX, origY)
}

// PulseCursor pulses the cursor to show an action happened.
func PulseCursor(x, y int) {
	origX, origY := robotgo.GetMousePos()

	// Three quick pulses
	for p := 0; p < 3; p++ {
		// Expand
		for r := 5; r <= 25; r += 5 {
			drawCircle(x, y, r)
			time.Sleep(10 * time.Millisecond)
		}
		// Contract
		for r := 25; r >= 5; r -= 5 {
			drawCircle(x, y, r)
			time.Sleep(10 * time.Millisecond)
		}
	}

	robotgo.Move(origX, origY)
}

func drawCircle(cx, cy, radius int) {
	steps := 36
	for i := 0; i <= steps; i++ {
		angle := float64(i) * 360.0 / float64(steps)
		rad := angle * 3.14159 / 180.0
		x := cx + int(float64(radius)*math.Cos(rad))
		y := cy + int(float64(radius)*math.Sin(rad))
		robotgo.Move(x, y)
	}
}
