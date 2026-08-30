package ocr

import (
	"image"
	"image/draw"
	"strings"
)

// ReadColorButtonRegions OCRs each proposed button rectangle as a tight crop
// at native scale (issue #158). Full-image layout analysis drops the large,
// uppercase, letter-spaced labels of saturated-colour buttons at every scale
// (see colored_buttons_diag_test.go), but a tight crop reads them at scale=1.
//
// Each crop is read twice — bright-text isolation (white labels on blue/green
// faces) and dark-text isolation (dark labels on yellow/orange faces) — as a
// single text line (PSM 7). Words are offset back to full-image coordinates
// and merged into one Result; duplicate reads of the same word from both
// isolations are collapsed.
func ReadColorButtonRegions(img image.Image, rects []image.Rectangle) (*Result, error) {
	if img == nil || len(rects) == 0 {
		return &Result{}, nil
	}
	bounds := img.Bounds()

	type wordKey struct {
		text string
		x, y int
	}
	seen := make(map[wordKey]bool)
	var words []Word
	var texts []string

	for _, r := range rects {
		r = r.Intersect(bounds)
		if r.Empty() {
			continue
		}
		crop := cropImage(img, r)

		for _, opts := range []Options{
			{BrightText: true, PageSegMode: 7},
			{DarkText: true, PageSegMode: 7},
		} {
			data, err := encodeForOCR(crop, 1, opts)
			if err != nil {
				continue
			}
			res, err := ReadPreparedBytes(data, 1, opts)
			if err != nil || res == nil {
				continue
			}
			for _, w := range res.Words {
				w.X += r.Min.X - bounds.Min.X
				w.Y += r.Min.Y - bounds.Min.Y
				key := wordKey{strings.ToLower(w.Text), (w.X / 10) * 10, (w.Y / 10) * 10}
				if seen[key] {
					continue
				}
				seen[key] = true
				words = append(words, w)
				texts = append(texts, w.Text)
			}
		}
	}

	return &Result{Text: strings.Join(texts, " "), Words: words}, nil
}

// cropImage returns the sub-rectangle of img with bounds translated to start
// at (0,0), so downstream preprocessing and Tesseract coordinates are
// crop-relative. SubImage alone is not enough: it keeps the original bounds,
// and Tesseract reports coordinates from the encoded pixel grid anyway.
func cropImage(img image.Image, r image.Rectangle) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(dst, dst.Bounds(), img, r.Min, draw.Src)
	return dst
}
