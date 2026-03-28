// Package ocr wraps Tesseract OCR via gosseract to extract text from images.
// Requires Tesseract OCR libraries to be installed (e.g., via vcpkg).
package ocr

import (
	"fmt"

	"github.com/otiai10/gosseract/v2"
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

// ReadFile runs OCR on the image at the given file path and returns structured output.
func ReadFile(imagePath string) (*Result, error) {
	client := gosseract.NewClient()
	defer client.Close()

	if err := client.SetImage(imagePath); err != nil {
		return nil, fmt.Errorf("set image: %w", err)
	}

	text, err := client.Text()
	if err != nil {
		return nil, fmt.Errorf("extract text: %w. Make sure Tesseract OCR is installed: https://github.com/tesseract-ocr/tesseract", err)
	}

	boxes, err := client.GetBoundingBoxes(gosseract.RIL_WORD)
	if err != nil {
		return nil, fmt.Errorf("get bounding boxes: %w", err)
	}

	words := make([]Word, 0, len(boxes))
	for _, b := range boxes {
		if b.Word == "" {
			continue
		}
		words = append(words, Word{
			Text:       b.Word,
			X:          b.Box.Min.X,
			Y:          b.Box.Min.Y,
			Width:      b.Box.Max.X - b.Box.Min.X,
			Height:     b.Box.Max.Y - b.Box.Min.Y,
			Confidence: b.Confidence,
		})
	}

	return &Result{Text: text, Words: words}, nil
}
