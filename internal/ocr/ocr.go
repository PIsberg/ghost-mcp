// Package ocr wraps Tesseract OCR via gosseract to extract text from images.
// Requires Tesseract OCR libraries to be installed (e.g., via vcpkg).
package ocr

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"

	"github.com/otiai10/gosseract/v2"
	"golang.org/x/image/draw"
)

// Word holds OCR-extracted text with its bounding box and confidence.
type Word struct {
	Text       string  `json:"text"`
	X          int     `json:"x"`
	Y          int     `json:"y"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Confidence float64 `json:"confidence"`
}

// Result holds the full OCR output for an image.
type Result struct {
	Text  string `json:"text"`
	Words []Word `json:"words"`
}

// Options controls OCR preprocessing behaviour.
type Options struct {
	// Color skips grayscale conversion and contrast stretching so that colour
	// information is preserved in the image sent to Tesseract. Use this when
	// the caller needs to distinguish elements by colour (e.g. "click the red
	// button"). Default false: grayscale + contrast stretch is applied, which
	// gives the best recognition accuracy for most UI text.
	Color bool
}

// MinConfidence is the minimum word confidence (0–100) to include in results.
// Words below this are OCR noise and should be discarded.
const MinConfidence = 30.0

// ScaleFactor is how much we upscale the image before OCR.
// Screen captures are ~96 DPI; Tesseract works best at ~300 DPI.
// 3x brings a 96 DPI capture to ~288 DPI, which sits in Tesseract's
// optimal range and measurably improves recognition of short UI text
// (button labels, menu items) compared to 2x. The extra memory cost
// (~9x pixels vs 4x) is acceptable for interactive use.
const ScaleFactor = 3

// ReadFile runs OCR on the image at the given file path and returns structured output.
func ReadFile(imagePath string, opts Options) (*Result, error) {
	// Preprocess and upscale the image before OCR for better accuracy.
	scaledPath, cleanup, err := scaleImage(imagePath, ScaleFactor, !opts.Color)
	if err != nil {
		// Non-fatal: fall back to original image.
		scaledPath = imagePath
		cleanup = func() {}
	}
	defer cleanup()

	client := gosseract.NewClient()
	defer client.Close()

	// PSM_SPARSE_TEXT finds text wherever it appears without imposing document
	// layout assumptions (columns, paragraphs). This is the right mode for UI
	// screenshots which contain scattered labels, buttons, and menu items rather
	// than structured prose.
	if err := client.SetPageSegMode(gosseract.PSM_SPARSE_TEXT); err != nil {
		return nil, fmt.Errorf("set page seg mode: %w", err)
	}

	if err := client.SetImage(scaledPath); err != nil {
		return nil, fmt.Errorf("set image: %w", err)
	}

	// GetBoundingBoxes triggers recognition internally via client.init().
	// The previous client.Text() call was a redundant second recognition pass
	// whose return value was discarded — removed for speed.
	boxes, err := client.GetBoundingBoxes(gosseract.RIL_WORD)
	if err != nil {
		return nil, fmt.Errorf("get bounding boxes: %w. Make sure Tesseract OCR is installed: https://github.com/tesseract-ocr/tesseract", err)
	}

	words := make([]Word, 0, len(boxes))
	for _, b := range boxes {
		if b.Word == "" || b.Confidence < MinConfidence {
			continue
		}
		words = append(words, Word{
			Text:       b.Word,
			X:          b.Box.Min.X / ScaleFactor,
			Y:          b.Box.Min.Y / ScaleFactor,
			Width:      (b.Box.Max.X - b.Box.Min.X) / ScaleFactor,
			Height:     (b.Box.Max.Y - b.Box.Min.Y) / ScaleFactor,
			Confidence: b.Confidence,
		})
	}

	// Reconstruct text from the filtered words so the text field matches words.
	var sb strings.Builder
	for i, w := range words {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(w.Text)
	}

	return &Result{Text: sb.String(), Words: words}, nil
}

// scaleImage preprocesses and upscales the image for OCR.
//
// When grayscale is true (the default), two preprocessing steps run before
// scaling:
//
//  1. Grayscale conversion (ITU-R BT.709 luminance weights) — removes
//     color-induced confusion. Tesseract is trained on grayscale and
//     silently converts color images itself, but explicit conversion lets
//     us also apply contrast stretching before that happens.
//
//  2. Linear contrast stretch — maps the darkest pixel to 0 and the
//     brightest to 255. This makes text pop against its background
//     regardless of the original palette (e.g. white text on a blue
//     button becomes near-white on near-black after stretching).
//
// When grayscale is false, colour is preserved and only the bilinear upscale
// is applied. Use this when the caller needs to distinguish elements by colour
// (e.g. "click the red button").
//
// Step 3 (always): bilinear upscale by factor — brings 96 DPI screen captures
// into Tesseract's optimal ~288 DPI range.
func scaleImage(src string, factor int, grayscale bool) (string, func(), error) {
	f, err := os.Open(src)
	if err != nil {
		return "", nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return "", nil, err
	}

	var scaledImg image.Image
	if grayscale {
		preprocessed := toGrayscaleContrast(img)
		b := preprocessed.Bounds()
		dst := image.NewGray(image.Rect(0, 0, b.Dx()*factor, b.Dy()*factor))
		draw.BiLinear.Scale(dst, dst.Bounds(), preprocessed, b, draw.Over, nil)
		scaledImg = dst
	} else {
		b := img.Bounds()
		dst := image.NewRGBA(image.Rect(0, 0, b.Dx()*factor, b.Dy()*factor))
		draw.BiLinear.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
		scaledImg = dst
	}

	tmp, err := os.CreateTemp("", "ghost-mcp-ocr-scaled-*.png")
	if err != nil {
		return "", nil, err
	}
	if err := png.Encode(tmp, scaledImg); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", nil, err
	}
	tmp.Close()

	cleanup := func() { os.Remove(tmp.Name()) }
	return tmp.Name(), cleanup, nil
}

// toGrayscaleContrast converts img to grayscale and applies linear contrast
// stretching. The result is an *image.Gray with pixel values mapped so that
// the darkest original pixel becomes 0 and the brightest becomes 255.
// If the image has less than 10 levels of variation (nearly uniform — e.g. a
// solid colour swatch) the stretch is skipped to avoid amplifying noise.
func toGrayscaleContrast(img image.Image) *image.Gray {
	b := img.Bounds()
	gray := image.NewGray(b)

	var minL, maxL uint8 = 255, 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA() // 16-bit per channel
			// ITU-R BT.709 luminance: 0.2126R + 0.7152G + 0.0722B
			// Shift right 8 to convert from 16-bit range to 8-bit.
			lum := (19595*r + 38470*g + 7471*bl + 1<<15) >> 24
			l := uint8(lum)
			gray.SetGray(x, y, color.Gray{Y: l})
			if l < minL {
				minL = l
			}
			if l > maxL {
				maxL = l
			}
		}
	}

	span := int(maxL) - int(minL)
	if span < 10 {
		// Nearly uniform image — contrast stretch would amplify noise.
		return gray
	}

	stretched := image.NewGray(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			l := gray.GrayAt(x, y).Y
			s := uint8((int(l-minL) * 255) / span)
			stretched.SetGray(x, y, color.Gray{Y: s})
		}
	}
	return stretched
}
