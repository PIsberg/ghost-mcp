package cv

import (
	"image"
)

// FindIcons applies a fast pure-Go edge detection and connected-component grouping
// to discover visual elements on the screen.
func FindIcons(img image.Image) []image.Rectangle {
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

	// 3. Connected Components on the Coarse Grid
	visited := make([]bool, gw*gh)
	var rects []image.Rectangle

	// Dilation step: if standard grid is used, sometimes icon pieces are 1 cell apart.
	// We'll link cells that are up to 2 cells apart (Chebyshev distance <= 2).

	for y := 0; y < gh; y++ {
		for x := 0; x < gw; x++ {
			if !grid[y*gw+x] || visited[y*gw+x] {
				continue
			}

			// BFS or DFS
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

				// Check neighborhood (radius 2 for dilation)
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

			// Convert back to pixel coordinates
			rect := image.Rect(
				minX*cellSize,
				minY*cellSize,
				(maxX+1)*cellSize,
				(maxY+1)*cellSize,
			)

			// Offset to global bounds
			rect = rect.Add(b.Min)

			// 4. Filtering criteria for icons
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
	}

	return rects
}
