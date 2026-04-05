// Package visual provides on-screen visual feedback for AI actions.
package visual

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"
	"time"

	"github.com/ghost-mcp/internal/learner"
	"github.com/go-vgo/robotgo"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// AnnotateImage draws bounding boxes and numeric ID badges on an image,
// facilitating AI visual reasoning (Set-of-Marks).
func AnnotateImage(src image.Image, elements []learner.Element) image.Image {
	bounds := src.Bounds()
	// Create an RGBA copy so we can draw safely.
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)

	// Colours for the annotations.
	boxColor := color.RGBA{0, 255, 0, 180}   // Bright green for boxes
	badgeColor := color.RGBA{0, 0, 0, 220}   // Dark background for text
	textColor := image.NewUniform(color.White)

	d := &font.Drawer{
		Dst:  dst,
		Src:  textColor,
		Face: basicfont.Face7x13,
	}

	for _, e := range elements {
		// 1. Draw element bounding box (1px outline)
		drawOutline(dst, e.X, e.Y, e.Width, e.Height, boxColor)

		// 2. Prepare the ID badge string
		idStr := strconv.Itoa(e.ID)
		textWidth := len(idStr) * 7
		badgeW := textWidth + 6
		badgeH := 16

		// 3. Draw badge background (positioned at top-left of element)
		badgeRect := image.Rect(e.X, e.Y-badgeH, e.X+badgeW, e.Y)
		// If at the very top of the screen, move badge inside the box
		if e.Y-badgeH < bounds.Min.Y {
			badgeRect = image.Rect(e.X, e.Y, e.X+badgeW, e.Y+badgeH)
		}
		draw.Draw(dst, badgeRect, image.NewUniform(badgeColor), image.Point{}, draw.Over)

		// 4. Draw the numeric ID
		d.Dot = fixed.Point26_6{
			X: fixed.I(badgeRect.Min.X + 3),
			Y: fixed.I(badgeRect.Min.Y + 12),
		}
		d.DrawString(idStr)
	}

	return dst
}

// drawOutline draws a 1-pixel rectangle outline.
func drawOutline(dst *image.RGBA, x, y, w, h int, c color.Color) {
	// Top and bottom
	for xOff := 0; xOff < w; xOff++ {
		dst.Set(x+xOff, y, c)
		dst.Set(x+xOff, y+h-1, c)
	}
	// Left and right
	for yOff := 0; yOff < h; yOff++ {
		dst.Set(x, y+yOff, c)
		dst.Set(x+w-1, y+yOff, c)
	}
}

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
