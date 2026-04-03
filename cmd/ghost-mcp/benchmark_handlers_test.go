//go:build !integration

package main

import (
	"image"
	"image/color"
	"testing"
)

// BenchmarkHashImageFast_FullHD measures the perceptual hash used for scroll
// deduplication on a typical full-HD screenshot.
func BenchmarkHashImageFast_FullHD(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	for y := 0; y < 1080; y++ {
		for x := 0; x < 1920; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x + y), A: 255})
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hashImageFast(img)
	}
}

// BenchmarkHashImageFast_Small measures the hash on a small viewport region.
func BenchmarkHashImageFast_Small(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 2), G: uint8(y * 2), B: 128, A: 255})
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hashImageFast(img)
	}
}

// BenchmarkHashImageFast_Uniform measures the degenerate case: a solid-color image
// (all pixels identical, hash loop still runs in full).
func BenchmarkHashImageFast_Uniform(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	for y := 0; y < 1080; y++ {
		for x := 0; x < 1920; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 128, G: 128, B: 128, A: 255})
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hashImageFast(img)
	}
}

// BenchmarkHashImageFast_Nil measures the nil-guard fast exit.
func BenchmarkHashImageFast_Nil(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hashImageFast(nil)
	}
}
