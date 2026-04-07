package main

import (
	"image"
	"image/color"
	"testing"
)

func TestComputeDHash_And_HammingDistance(t *testing.T) {
	// Create a simple mostly-white image
	img1 := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if x > 50 {
				img1.Set(x, y, color.RGBA{255, 255, 255, 255})
			} else {
				img1.Set(x, y, color.RGBA{0, 0, 0, 255})
			}
		}
	}

	hash1 := computeDHash(img1)

	// Create an identical image
	img2 := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if x > 50 {
				img2.Set(x, y, color.RGBA{255, 255, 255, 255})
			} else {
				img2.Set(x, y, color.RGBA{0, 0, 0, 255})
			}
		}
	}
	hash2 := computeDHash(img2)

	// Add noise to a third image
	img3 := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if x > 50 {
				// slight noise shouldn't affect dHash heavily
				img3.Set(x, y, color.RGBA{250, 250, 250, 255})
			} else {
				img3.Set(x, y, color.RGBA{5, 5, 5, 255})
			}
		}
	}
	hash3 := computeDHash(img3)

	// Create a very different image
	img4 := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img4.Set(x, y, color.RGBA{128, 128, 128, 255})
		}
	}
	hash4 := computeDHash(img4)

	dist12 := hammingDistance(hash1, hash2)
	dist13 := hammingDistance(hash1, hash3)
	dist14 := hammingDistance(hash1, hash4)

	if dist12 != 0 {
		t.Errorf("Expected identical images to have distance 0, got %d", dist12)
	}

	if dist13 > 2 {
		t.Errorf("Expected noisy image to have very low distance from original, got %d", dist13)
	}

	if dist14 < 5 {
		t.Errorf("Expected radically different image to have higher distance, got %d", dist14)
	}
}
