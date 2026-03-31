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

	"github.com/otiai10/gosseract/v2"
	"golang.org/x/image/bmp"
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

	// BrightText isolates near-white pixels for detecting white text on any
	// coloured or dark background. Each pixel is mapped to black if all three
	// RGB channels are ≥ 240, otherwise to white. The result is a high-contrast
	// binary image where pure-white button labels appear as black text on a
	// white background — the pattern Tesseract is trained on. Threshold 240
	// captures true white text while excluding near-white body text (e.g.
	// #eee = 238) and all coloured backgrounds. Use as a last-resort fallback
	// when grayscale, inverted, and colour passes all fail. Ignored when Color
	// or Inverted is true.
	BrightText bool
}

// MinConfidence is the minimum word confidence (0–100) to include in results.
// Words below this are OCR noise and should be discarded.
const MinConfidence = 30.0

// We instantiate a new Tesseract client for every invocation dynamically instead
// of holding a global client. This intentionally avoids maintaining a permanent 
// 800MB RAM footprint and allows true parallel goroutine execution without
// Mutex locks crashing the underlying C++ pointer references.
func newClient() (*gosseract.Client, error) {
	c := gosseract.NewClient()
	if err := c.SetPageSegMode(gosseract.PSM_SPARSE_TEXT); err != nil {
		c.Close()
		return nil, fmt.Errorf("init tesseract page seg mode: %w", err)
	}
	return c, nil
}

// ScaleFactor is how much we upscale the image before OCR.
// Screen captures are ~96 DPI; Tesseract works best at ~300 DPI.
// 3x brings a 96 DPI capture to ~288 DPI, which sits in Tesseract's
// optimal range and measurably improves recognition of short UI text
// (button labels, menu items) compared to 2x. The extra memory cost
// (~9x pixels vs 4x) is acceptable for interactive use.
const ScaleFactor = 3

func ReadFile(imagePath string, opts Options) (*Result, error) {
	client, err := newClient()
	if err != nil {
		return nil, fmt.Errorf("tesseract unavailable: %w. Make sure Tesseract OCR is installed: https://github.com/tesseract-ocr/tesseract", err)
	}
	defer client.Close()

	// Preprocess + upscale into an in-memory PNG buffer and hand it to Tesseract
	// via SetImageFromBytes. This avoids writing a temp file to disk and the
	// subsequent disk read by Tesseract — two fewer disk round trips per call.
	imgBytes, prepErr := preprocessToBytes(imagePath, ScaleFactor, opts)
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
	client, err := newClient()
	if err != nil {
		return nil, fmt.Errorf("tesseract unavailable: %w. Make sure Tesseract OCR is installed: https://github.com/tesseract-ocr/tesseract", err)
	}
	defer client.Close()

	imgBytes, prepErr := encodeForOCR(img, ScaleFactor, opts)
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
func preprocessToBytes(src string, factor int, opts Options) ([]byte, error) {
	f, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}

	return encodeForOCR(img, factor, opts)
}

// encodeForOCR applies preprocessing to img and returns PNG-encoded bytes
// suitable for Tesseract. It is the shared core used by both preprocessToBytes
// (file path input) and ReadImage (in-memory image input).
//
// Preprocessing is selected by opts (evaluated in priority order):
//  1. BrightText: pixels where all RGB ≥ 240 → black, else → white.
//     Isolates pure-white button text from any background. Upscaled as Gray.
//  2. Color (and not BrightText): no grayscale/inversion; upscale as RGBA.
//  3. Default (grayscale): BT.709 grayscale + contrast stretch + optional
//     inversion (opts.Inverted). Upscaled as Gray.
func encodeForOCR(img image.Image, factor int, opts Options) ([]byte, error) {
	var scaledImg image.Image
	if opts.BrightText {
		preprocessed := brightTextToGray(img, 240)
		b := preprocessed.Bounds()
		dst := image.NewGray(image.Rect(0, 0, b.Dx()*factor, b.Dy()*factor))
		draw.NearestNeighbor.Scale(dst, dst.Bounds(), preprocessed, b, draw.Over, nil)
		scaledImg = dst
	} else if opts.Color {
		b := img.Bounds()
		dst := image.NewRGBA(image.Rect(0, 0, b.Dx()*factor, b.Dy()*factor))
		draw.NearestNeighbor.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
		scaledImg = dst
	} else {
		preprocessed := toGrayscaleContrast(img)
		if opts.Inverted {
			invertGray(preprocessed)
		}
		b := preprocessed.Bounds()
		dst := image.NewGray(image.Rect(0, 0, b.Dx()*factor, b.Dy()*factor))
		draw.NearestNeighbor.Scale(dst, dst.Bounds(), preprocessed, b, draw.Over, nil)
		scaledImg = dst
	}

	var buf bytes.Buffer
	format := strings.ToLower(os.Getenv("GHOST_MCP_OCR_FORMAT"))
	if format == "png" {
		if err := png.Encode(&buf, scaledImg); err != nil {
			return nil, err
		}
	} else {
		// Default to raw uncompressed BMP for ultra-fast generation
		if err := bmp.Encode(&buf, scaledImg); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// invertGray flips every pixel in-place: new = 255 - old.
func invertGray(img *image.Gray) {
	for i, v := range img.Pix {
		img.Pix[i] = 255 - v
	}
}

// brightTextToGray creates a binary image where pixels with all three RGB
// channels at or above threshold become black, and all other pixels become
// white. This reliably isolates white (or near-white) text from any coloured
// or dark background by exploiting the fact that pure white has all channels
// uniformly high, while coloured backgrounds have at least one low channel.
//
// Result: black text on white background — the pattern Tesseract is trained on.
//
// threshold=240 is chosen to capture true white button text (255,255,255)
// while excluding near-white body text like #eee (238,238,238) and any
// coloured background gradient (always has at least one channel < 240).
func brightTextToGray(img image.Image, threshold uint8) *image.Gray {
	b := img.Bounds()
	out := image.NewGray(b)
	w, h := b.Dx(), b.Dy()

	// Fast path for *image.RGBA — direct Pix slice access, no interface calls.
	if rgba, ok := img.(*image.RGBA); ok {
		for y := 0; y < h; y++ {
			srcRow := rgba.Pix[y*rgba.Stride : y*rgba.Stride+w*4]
			dstOff := y * out.Stride
			for x := 0; x < w; x++ {
				r := srcRow[x*4]
				g := srcRow[x*4+1]
				bl := srcRow[x*4+2]
				if r >= threshold && g >= threshold && bl >= threshold {
					out.Pix[dstOff+x] = 0 // near-white → black (text)
				} else {
					out.Pix[dstOff+x] = 255 // everything else → white (background)
				}
			}
		}
	} else {
		// Slow path: interface dispatch fallback for any other image type.
		for y := b.Min.Y; y < b.Max.Y; y++ {
			dstOff := (y-b.Min.Y)*out.Stride - b.Min.X
			for x := b.Min.X; x < b.Max.X; x++ {
				r32, g32, b32, _ := img.At(x, y).RGBA()
				r8, g8, b8 := uint8(r32>>8), uint8(g32>>8), uint8(b32>>8)
				if r8 >= threshold && g8 >= threshold && b8 >= threshold {
					out.Pix[dstOff+x] = 0
				} else {
					out.Pix[dstOff+x] = 255
				}
			}
		}
	}
	return out
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
