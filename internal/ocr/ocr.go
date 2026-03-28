// Package ocr wraps Tesseract OCR via gosseract to extract text from images.
// Requires Tesseract OCR libraries to be installed (e.g., via vcpkg).
package ocr

import (
	"fmt"
	"image"
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

// MinConfidence is the minimum word confidence (0–100) to include in results.
// Words below this are OCR noise and should be discarded.
const MinConfidence = 30.0

// ScaleFactor is how much we upscale the image before OCR.
// Screen captures are ~96 DPI; Tesseract works best at 300 DPI.
// 3x brings 96 DPI close enough to 300 DPI for reliable results.
const ScaleFactor = 3

// ReadFile runs OCR on the image at the given file path and returns structured output.
func ReadFile(imagePath string) (*Result, error) {
	// Upscale the image before OCR for better accuracy on screen captures.
	scaledPath, cleanup, err := scaleImage(imagePath, ScaleFactor)
	if err != nil {
		// Non-fatal: fall back to original image.
		scaledPath = imagePath
		cleanup = func() {}
	}
	defer cleanup()

	client := gosseract.NewClient()
	defer client.Close()

	if err := client.SetImage(scaledPath); err != nil {
		return nil, fmt.Errorf("set image: %w", err)
	}

	_, err = client.Text()
	if err != nil {
		return nil, fmt.Errorf("extract text: %w. Make sure Tesseract OCR is installed: https://github.com/tesseract-ocr/tesseract", err)
	}

	boxes, err := client.GetBoundingBoxes(gosseract.RIL_WORD)
	if err != nil {
		return nil, fmt.Errorf("get bounding boxes: %w", err)
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

// scaleImage writes a scaled copy of the image to a temp file and returns
// its path along with a cleanup function. The caller must call cleanup().
func scaleImage(src string, factor int) (string, func(), error) {
	f, err := os.Open(src)
	if err != nil {
		return "", nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return "", nil, err
	}

	b := img.Bounds()
	scaled := image.NewRGBA(image.Rect(0, 0, b.Dx()*factor, b.Dy()*factor))
	draw.BiLinear.Scale(scaled, scaled.Bounds(), img, b, draw.Over, nil)

	tmp, err := os.CreateTemp("", "ghost-mcp-ocr-scaled-*.png")
	if err != nil {
		return "", nil, err
	}
	if err := png.Encode(tmp, scaled); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", nil, err
	}
	tmp.Close()

	cleanup := func() { os.Remove(tmp.Name()) }
	return tmp.Name(), cleanup, nil
}
