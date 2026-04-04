//go:build !integration

package main

import (
	"context"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"runtime"
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

func BenchmarkParallelFindText_FixtureButtons_Grayscale(b *testing.B) {
	img := benchmarkLoadImage(b, "docs/screenshots/01-initial-fixture.png")
	ctx := context.Background()
	searchText := "Button"

	if _, _, _, _, found, _ := parallelFindText(ctx, img, searchText, 1, true, ""); !found {
		b.Fatalf("expected to find %q in fixture screenshot", searchText)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, _, _, found, _ := parallelFindText(ctx, img, searchText, 1, true, ""); !found {
			b.Fatalf("expected to find %q in fixture screenshot", searchText)
		}
	}
}

func BenchmarkParallelFindText_FixtureButtons_ColorOnly(b *testing.B) {
	img := benchmarkLoadImage(b, "docs/screenshots/01-initial-fixture.png")
	ctx := context.Background()
	searchText := "Button"

	if _, _, _, _, found, _ := parallelFindText(ctx, img, searchText, 1, false, ""); !found {
		b.Fatalf("expected to find %q in fixture screenshot", searchText)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, _, _, found, _ := parallelFindText(ctx, img, searchText, 1, false, ""); !found {
			b.Fatalf("expected to find %q in fixture screenshot", searchText)
		}
	}
}
