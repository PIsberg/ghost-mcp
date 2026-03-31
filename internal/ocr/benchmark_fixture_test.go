package ocr

import (
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func benchmarkRepoRoot(tb testing.TB) string {
	tb.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		tb.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func benchmarkLoadImage(tb testing.TB, relPath string) image.Image {
	tb.Helper()
	path := filepath.Join(benchmarkRepoRoot(tb), filepath.FromSlash(relPath))
	f, err := os.Open(path)
	if err != nil {
		tb.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		tb.Fatalf("decode %s: %v", path, err)
	}
	return img
}

func benchmarkRequireTesseract(b *testing.B) {
	b.Helper()
	img := whiteImage(64, 32)
	if _, err := ReadImage(img, Options{}); err != nil {
		if strings.Contains(err.Error(), "tessdata") {
			b.Skip("Tesseract data files not available")
		}
	}
}

func BenchmarkReadImage_FixturePanel_Grayscale(b *testing.B) {
	benchmarkRequireTesseract(b)
	img := benchmarkLoadImage(b, "docs/screenshots/05-ocr-panel.png")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := ReadImage(img, Options{})
		if err != nil {
			b.Fatalf("ReadImage: %v", err)
		}
		if len(result.Words) == 0 {
			b.Fatal("expected OCR words from fixture panel")
		}
	}
}

func BenchmarkReadImage_FixturePanel_Color(b *testing.B) {
	benchmarkRequireTesseract(b)
	img := benchmarkLoadImage(b, "docs/screenshots/05-ocr-panel.png")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := ReadImage(img, Options{Color: true})
		if err != nil {
			b.Fatalf("ReadImage: %v", err)
		}
		if len(result.Words) == 0 {
			b.Fatal("expected OCR words from fixture panel")
		}
	}
}
