package cv

import (
	"image"
)

// analysis holds the shared edge-grid connected-component result plus the
// grayscale buffer, so shape detectors can run cheap interior checks without
// re-scanning the image.
type analysis struct {
	rects  []image.Rectangle // component bounding boxes, in img's coordinate space
	gray   []uint8           // grayscale pixels, indexed [y*w+x] relative to bounds.Min
	w, h   int
	bounds image.Rectangle
}

// analyzeComponents applies a fast pure-Go edge detection and
// connected-component grouping over a coarse grid. It returns every component
// found, unfiltered; detectors like FindIcons and FindInputBoxes apply their
// own shape criteria.
func analyzeComponents(img image.Image) *analysis {
	if img == nil {
		return nil
	}

	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	// High-contrast edge detection parameters
	const cellSize = 6
	const edgeThreshold = 35 // intensity difference (0-255)

	gw := w / cellSize
	gh := h / cellSize
	if gw == 0 || gh == 0 {
		return nil
	}

	// 1. Grayscale extraction (faster array access)
	gray := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bCol, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			gray[y*w+x] = uint8((r*299 + g*587 + bCol*114) / 256000)
		}
	}

	// 2. Coarse grid edge detection
	grid := make([]bool, gw*gh)

	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			idx := y*w + x
			p := gray[idx]

			// Simple cross difference
			dx := int(p) - int(gray[idx-1])
			if dx < 0 {
				dx = -dx
			}
			dy := int(p) - int(gray[idx-w])
			if dy < 0 {
				dy = -dy
			}

			if dx > edgeThreshold || dy > edgeThreshold {
				cx, cy := x/cellSize, y/cellSize
				if cx < gw && cy < gh {
					grid[cy*gw+cx] = true
				}
			}
		}
	}

	// 3. Connected components on the coarse grid.
	// Dilation: link cells up to 2 cells apart (Chebyshev distance <= 2) so
	// icon pieces separated by a small gap still group together.
	visited := make([]bool, gw*gh)
	var rects []image.Rectangle

	for y := 0; y < gh; y++ {
		for x := 0; x < gw; x++ {
			if !grid[y*gw+x] || visited[y*gw+x] {
				continue
			}

			minX, maxX := x, x
			minY, maxY := y, y
			q := []int{y*gw + x}
			visited[y*gw+x] = true

			for len(q) > 0 {
				curr := q[0]
				q = q[1:]
				cy := curr / gw
				cx := curr % gw

				if cx < minX {
					minX = cx
				}
				if cx > maxX {
					maxX = cx
				}
				if cy < minY {
					minY = cy
				}
				if cy > maxY {
					maxY = cy
				}

				for ny := cy - 2; ny <= cy+2; ny++ {
					for nx := cx - 2; nx <= cx+2; nx++ {
						if nx >= 0 && nx < gw && ny >= 0 && ny < gh {
							nIdx := ny*gw + nx
							if grid[nIdx] && !visited[nIdx] {
								visited[nIdx] = true
								q = append(q, nIdx)
							}
						}
					}
				}
			}

			rect := image.Rect(
				minX*cellSize,
				minY*cellSize,
				(maxX+1)*cellSize,
				(maxY+1)*cellSize,
			)
			rects = append(rects, rect.Add(b.Min))
		}
	}

	return &analysis{rects: rects, gray: gray, w: w, h: h, bounds: b}
}

// FindIcons discovers icon-sized visual elements: compact components that are
// neither text-line thin nor panel large.
func FindIcons(img image.Image) []image.Rectangle {
	a := analyzeComponents(img)
	if a == nil {
		return nil
	}

	var rects []image.Rectangle
	for _, rect := range a.rects {
		rw, rh := rect.Dx(), rect.Dy()

		// Too small? (noise or 1-letter text)
		if rw < 14 || rh < 14 {
			continue
		}
		// Too big? (panels, windows, large hero images)
		if rw > 150 || rh > 150 {
			continue
		}
		// Extreme aspect ratios (lines, dividers, text inputs)
		if rw > rh*4 || rh > rw*4 {
			continue
		}

		rects = append(rects, rect)
	}
	return rects
}

// FindInputBoxes discovers rectangles that look like text-entry fields: wide,
// short components whose interior is mostly light and uniform, the way
// browsers render <input> and <textarea> backgrounds. OCR cannot see an input
// that contains no text at all, so this shape detector is what gives the
// learned view coordinates (and therefore visual_ids) for empty fields.
func FindInputBoxes(img image.Image) []image.Rectangle {
	a := analyzeComponents(img)
	if a == nil {
		return nil
	}

	var rects []image.Rectangle
	for _, rect := range a.rects {
		rw, rh := rect.Dx(), rect.Dy()

		// Single-line inputs are ~20-50px tall, textareas up to ~130px.
		if rh < 18 || rh > 130 {
			continue
		}
		// Wide and clearly wider than tall (3:1) — separates fields from
		// icons, cards, and swatches.
		if rw < 80 || rw < rh*3 {
			continue
		}
		if !a.lightInterior(rect) {
			continue
		}
		rects = append(rects, rect)
	}
	return rects
}

// lightInterior reports whether the inset interior of r is predominantly
// bright, as input-field backgrounds are white to light gray. The 85%
// tolerance leaves room for a caret, placeholder text, or a label that the
// coarse grid merged into the component.
func (a *analysis) lightInterior(r image.Rectangle) bool {
	const inset = 5
	const brightMin = 200
	const requiredRatio = 0.85

	interior := image.Rect(r.Min.X+inset, r.Min.Y+inset, r.Max.X-inset, r.Max.Y-inset)
	interior = interior.Intersect(a.bounds)
	if interior.Empty() {
		return false
	}

	bright, total := 0, 0
	for y := interior.Min.Y; y < interior.Max.Y; y += 3 {
		for x := interior.Min.X; x < interior.Max.X; x += 3 {
			lx, ly := x-a.bounds.Min.X, y-a.bounds.Min.Y
			if lx < 0 || ly < 0 || lx >= a.w || ly >= a.h {
				continue
			}
			total++
			if a.gray[ly*a.w+lx] >= brightMin {
				bright++
			}
		}
	}
	if total == 0 {
		return false
	}
	return float64(bright)/float64(total) >= requiredRatio
}
