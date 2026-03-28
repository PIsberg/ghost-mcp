//go:build noocr

// Package ocr provides a stub implementation when Tesseract is not available.
// To enable OCR support, build with: go build -tags ocr
package ocr

import (
	"fmt"
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

// ReadFile returns an error indicating OCR is not available.
func ReadFile(imagePath string) (*Result, error) {
	return nil, fmt.Errorf("OCR functionality is not enabled in this build. To enable OCR, rebuild with: go build -tags ocr")
}
