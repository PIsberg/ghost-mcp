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
	"github.com/ghost-mcp/internal/logging"
	"github.com/go-vgo/robotgo"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// AnnotateImage draws bounding boxes and numeric ID badges on an image,
// facilitating AI visual reasoning (Set-of-Marks).
// offsetX and offsetY are the top-left coordinates of the src image in absolute screen space.
// dpiScale is used to thicken borders and badges on high-DPI displays.
func AnnotateImage(src image.Image, elements []learner.Element, offsetX, offsetY int, dpiScale float64) image.Image {
	if dpiScale <= 0 {
		dpiScale = 1.0
	}
	bounds := src.Bounds()
	logging.Info("[VISUAL] Annotating image: bounds=%v, offset=(%d,%d), scale=%.2f, elements=%d", bounds, offsetX, offsetY, dpiScale, len(elements))

	// Create an RGBA copy so we can draw safely.
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)

	// Colours for the annotations.
	boxColor := color.RGBA{0, 255, 0, 180} // Bright green for boxes
	badgeColor := color.RGBA{0, 0, 0, 220} // Dark background for text
	textColor := image.NewUniform(color.White)

	d := &font.Drawer{
		Dst:  dst,
		Src:  textColor,
		Face: basicfont.Face7x13,
	}

	thickness := int(math.Max(1.0, math.Round(dpiScale)))

	for i, e := range elements {
		// Normalize coordinates to the image's local space
		// relX is relative to the start of the screenshot (offsetX)
		relX := e.X - offsetX
		relY := e.Y - offsetY

		// Convert to absolute pixel space in the destination image
		absX := relX + bounds.Min.X
		absY := relY + bounds.Min.Y

		// Skip if the element is completely outside the capture region
		if absX+e.Width < bounds.Min.X || absX > bounds.Max.X ||
			absY+e.Height < bounds.Min.Y || absY > bounds.Max.Y {
			continue
		}

		if i < 5 {
			logging.Debug("[VISUAL] Element %d: ID=%d, screen=(%d,%d), local_rel=(%d,%d), dest_abs=(%d,%d)", i, e.ID, e.X, e.Y, relX, relY, absX, absY)
		}

		// 1. Draw element bounding box
		drawOutline(dst, absX, absY, e.Width, e.Height, thickness, boxColor)

		// 2. Prepare the ID badge string
		idStr := strconv.Itoa(e.ID)
		textWidth := len(idStr) * 7
		badgeW := textWidth + 6
		badgeH := 16

		// Use larger badge if on high-DPI
		if dpiScale > 1.25 {
			badgeW = int(float64(badgeW) * dpiScale)
			badgeH = int(float64(badgeH) * dpiScale)
		}

		// 3. Draw badge background (positioned at top-left of element)
		badgeRect := image.Rect(absX, absY-badgeH, absX+badgeW, absY)
		// If at the very top of the image region, move badge inside the box
		if absY-badgeH < bounds.Min.Y {
			badgeRect = image.Rect(absX, absY, absX+badgeW, absY+badgeH)
		}
		draw.Draw(dst, badgeRect, image.NewUniform(badgeColor), image.Point{}, draw.Over)

		// 4. Draw the numeric ID
		// Center the text in the badge
		dotY := badgeRect.Min.Y + 12
		if dpiScale > 1.25 {
			dotY = badgeRect.Min.Y + (badgeH+13)/2 - 2
		}
		d.Dot = fixed.Point26_6{
			X: fixed.I(badgeRect.Min.X + (badgeRect.Dx()-textWidth)/2),
			Y: fixed.I(dotY),
		}
		d.DrawString(idStr)
	}

	return dst
}

// drawOutline draws a rectangle outline with a given thickness.
func drawOutline(dst *image.RGBA, x, y, w, h, thickness int, c color.Color) {
	for t := 0; t < thickness; t++ {
		tx, ty, tw, th := x+t, y+t, w-2*t, h-2*t
		if tw <= 0 || th <= 0 {
			break
		}
		// Top and bottom
		for xOff := 0; xOff < tw; xOff++ {
			dst.Set(tx+xOff, ty, c)
			dst.Set(tx+xOff, ty+th-1, c)
		}
		// Left and right
		for yOff := 0; yOff < th; yOff++ {
			dst.Set(tx, ty+yOff, c)
			dst.Set(tx+tw-1, ty+yOff, c)
		}
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
