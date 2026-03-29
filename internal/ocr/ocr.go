// Package ocr wraps Tesseract OCR via gosseract to extract text from images.
// Requires Tesseract OCR libraries to be installed (e.g., via vcpkg).
package ocr

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"
	"sync"

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

	// Inverted flips pixel brightness (255-x) after grayscale conversion.
	// Use this as a fallback when white text on a dark/coloured background
	// is not detected: CSS buttons with white text on gradient backgrounds
	// are invisible to Tesseract after normal preprocessing because the white
	// page background and white button text both map to the same brightness.
	// Inversion makes button text dark on a lighter button background, which
	// is what Tesseract is trained on. Ignored when Color is true.
	Inverted bool
}

// MinConfidence is the minimum word confidence (0–100) to include in results.
// Words below this are OCR noise and should be discarded.
const MinConfidence = 30.0

// sharedClient is a process-wide Tesseract client. Tesseract's Init() loads
// trained data files from disk (~100–500 ms on first call). By reusing one
// client across all ReadFile calls we pay that cost once per process lifetime
// instead of once per OCR request.
//
// gosseract's internal init() guard (shouldInit flag) ensures tessdata is only
// loaded on the first call; subsequent calls just call SetPixImage + Recognize.
//
// sharedClientMu serialises access because the Tesseract C++ API is not
// thread-safe.
var (
	sharedClient    *gosseract.Client
	sharedClientMu  sync.Mutex
	sharedClientErr error
	sharedClientOnce sync.Once
)

// getClient returns the process-wide Tesseract client, initialising it on the
// first call. Returns an error if Tesseract could not be initialised.
func getClient() (*gosseract.Client, error) {
	sharedClientOnce.Do(func() {
		c := gosseract.NewClient()
		if err := c.SetPageSegMode(gosseract.PSM_SPARSE_TEXT); err != nil {
			c.Close()
			sharedClientErr = fmt.Errorf("init tesseract page seg mode: %w", err)
			return
		}
		sharedClient = c
	})
	return sharedClient, sharedClientErr
}

// ScaleFactor is how much we upscale the image before OCR.
// Screen captures are ~96 DPI; Tesseract works best at ~300 DPI.
// 3x brings a 96 DPI capture to ~288 DPI, which sits in Tesseract's
// optimal range and measurably improves recognition of short UI text
// (button labels, menu items) compared to 2x. The extra memory cost
// (~9x pixels vs 4x) is acceptable for interactive use.
const ScaleFactor = 3

// ReadFile runs OCR on the image at the given file path and returns structured output.
func ReadFile(imagePath string, opts Options) (*Result, error) {
	client, err := getClient()
	if err != nil {
		return nil, fmt.Errorf("tesseract unavailable: %w. Make sure Tesseract OCR is installed: https://github.com/tesseract-ocr/tesseract", err)
	}

	// Serialise: Tesseract C++ API is not thread-safe.
	sharedClientMu.Lock()
	defer sharedClientMu.Unlock()

	// Preprocess + upscale into an in-memory PNG buffer and hand it to Tesseract
	// via SetImageFromBytes. This avoids writing a temp file to disk and the
	// subsequent disk read by Tesseract — two fewer disk round trips per call.
	imgBytes, prepErr := preprocessToBytes(imagePath, ScaleFactor, !opts.Color, opts.Inverted)
	if prepErr == nil {
		if err := client.SetImageFromBytes(imgBytes); err != nil {
			return nil, fmt.Errorf("set image from bytes: %w", err)
		}
	} else {
		// Non-fatal: preprocessing failed (rare — decode error, OOM).
		// Fall back to the original unprocessed file.
		if err := client.SetImage(imagePath); err != nil {
			return nil, fmt.Errorf("set image: %w", err)
		}
	}

	// GetBoundingBoxes triggers recognition. On the first call client.init()
	// runs the full Tesseract Init (loads tessdata). On all subsequent calls
	// shouldInit==false so init() only sets the new pixel image — tessdata is
	// already loaded in memory.
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

// ReadImage runs OCR on an already-decoded image and returns structured output.
// It skips all file I/O — useful when the caller already has an image in memory
// (e.g. from a screen capture) and does not want to pay the cost of writing a
// temp file just to read it back.
func ReadImage(img image.Image, opts Options) (*Result, error) {
	client, err := getClient()
	if err != nil {
		return nil, fmt.Errorf("tesseract unavailable: %w. Make sure Tesseract OCR is installed: https://github.com/tesseract-ocr/tesseract", err)
	}

	sharedClientMu.Lock()
	defer sharedClientMu.Unlock()

	imgBytes, prepErr := encodeForOCR(img, ScaleFactor, !opts.Color, opts.Inverted)
	if prepErr == nil {
		if err := client.SetImageFromBytes(imgBytes); err != nil {
			return nil, fmt.Errorf("set image from bytes: %w", err)
		}
	} else {
		return nil, fmt.Errorf("preprocess image: %w", prepErr)
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

	var sb strings.Builder
	for i, w := range words {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(w.Text)
	}
	return &Result{Text: sb.String(), Words: words}, nil
}

// preprocessToBytes loads the image at src, applies optional preprocessing,
// upscales by factor, and returns PNG-encoded bytes ready for Tesseract.
// Using bytes avoids writing a temp file and the subsequent Tesseract disk read.
func preprocessToBytes(src string, factor int, grayscale, inverted bool) ([]byte, error) {
	f, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}

	return encodeForOCR(img, factor, grayscale, inverted)
}

// encodeForOCR applies preprocessing to img and returns PNG-encoded bytes
// suitable for Tesseract. It is the shared core used by both preprocessToBytes
// (file path input) and ReadImage (in-memory image input).
//
// When grayscale is true:
//  1. Grayscale conversion (ITU-R BT.709) + linear contrast stretch.
//  2. Inversion (255-x) — only when inverted is true.
//  3. Bilinear upscale by factor.
//
// When grayscale is false colour is preserved and only the upscale is applied.
func encodeForOCR(img image.Image, factor int, grayscale, inverted bool) ([]byte, error) {
	var scaledImg image.Image
	if grayscale {
		preprocessed := toGrayscaleContrast(img)
		if inverted {
			invertGray(preprocessed)
		}
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

	var buf bytes.Buffer
	if err := png.Encode(&buf, scaledImg); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// invertGray flips every pixel in-place: new = 255 - old.
func invertGray(img *image.Gray) {
	for i, v := range img.Pix {
		img.Pix[i] = 255 - v
	}
}

// toGrayscaleContrast converts img to grayscale and applies linear contrast
// stretching in a single allocation. The result is an *image.Gray with pixel
// values mapped so that the darkest original pixel becomes 0 and the brightest
// becomes 255. If the image has less than 10 levels of variation (nearly
// uniform — e.g. a solid colour swatch) the stretch is skipped to avoid
// amplifying noise.
//
// Performance notes:
//   - *image.RGBA (the type returned by robotgo) takes a fast path that reads
//     the underlying Pix byte slice directly, avoiding per-pixel interface
//     dispatch and the intermediate RGBA() conversion.
//   - Contrast stretch is applied in-place on the single output Gray.Pix
//     slice, so no second allocation is needed even when stretching.
func toGrayscaleContrast(img image.Image) *image.Gray {
	b := img.Bounds()
	out := image.NewGray(b)
	w, h := b.Dx(), b.Dy()

	var minL, maxL uint8 = 255, 0

	// Fast path for *image.RGBA — direct Pix slice access, no interface calls.
	if rgba, ok := img.(*image.RGBA); ok {
		for y := 0; y < h; y++ {
			srcRow := rgba.Pix[y*rgba.Stride : y*rgba.Stride+w*4]
			dstOff := y * out.Stride
			for x := 0; x < w; x++ {
				r := uint32(srcRow[x*4])
				g := uint32(srcRow[x*4+1])
				bl := uint32(srcRow[x*4+2])
				// ITU-R BT.709 coefficients for 8-bit input → 8-bit output.
				// (19595 + 38470 + 7471) == 65536, so white maps to exactly 255.
				l := uint8((19595*r + 38470*g + 7471*bl + 1<<15) >> 16)
				out.Pix[dstOff+x] = l
				if l < minL {
					minL = l
				}
				if l > maxL {
					maxL = l
				}
			}
		}
	} else {
		// Slow path: interface dispatch fallback for any other image type.
		for y := b.Min.Y; y < b.Max.Y; y++ {
			dstOff := (y-b.Min.Y)*out.Stride - b.Min.X
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, bl, _ := img.At(x, y).RGBA() // 16-bit per channel
				lum := (19595*r + 38470*g + 7471*bl + 1<<15) >> 24
				l := uint8(lum)
				out.Pix[dstOff+x] = l
				if l < minL {
					minL = l
				}
				if l > maxL {
					maxL = l
				}
			}
		}
	}

	span := int(maxL) - int(minL)
	if span < 10 {
		// Nearly uniform image — contrast stretch would amplify noise.
		return out
	}

	// In-place contrast stretch on the single output Pix slice — no second allocation.
	for i, l := range out.Pix {
		out.Pix[i] = uint8((int(l-minL) * 255) / span)
	}
	return out
}
