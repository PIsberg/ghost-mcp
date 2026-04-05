// Package ocr wraps Tesseract OCR via gosseract to extract text from images.
// Requires Tesseract OCR libraries to be installed (e.g., via vcpkg).
package ocr

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"sync"

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
	// coloured or dark background. A pixel is mapped to black (text) when its
	// BT.709 luminance ≥ 185 AND its channel spread (max−min) ≤ 130; everything
	// else becomes white (background). The dual condition handles gradients like
	// the WARNING button (#f093fb→#f5576c) where a min-channel threshold fails:
	// a 50%-blended anti-aliased edge has lum=188.5 ≥ 185 while the pure pink
	// start colour has lum=174.3 < 185, giving a clean gap. The spread cap
	// (brightTextMaxSpread=130) additionally excludes high-luma coloured
	// backgrounds such as cyan #00f2fe (spread=254) and bright green #38ef7d
	// (spread=183). Use as a fallback when grayscale, inverted, and colour
	// passes all fail. Ignored when Color or Inverted is true.
	BrightText bool

	// ColorInvert flips the RGB image (255−R, 255−G, 255−B) BEFORE converting
	// to grayscale. This makes white text black and coloured backgrounds a
	// medium-to-dark gray — producing high-contrast dark-on-light text that
	// Tesseract is trained on. Unlike the regular Inverted pass (grayscale THEN
	// invert, which yields dark-on-darker with low contrast), this pass works
	// for light coloured backgrounds such as the cyan INFO button (#4facfe→#00f2fe)
	// where the luminance gap between white text and background is too small
	// after grayscale conversion.
	ColorInverted bool

	// DarkText isolates near-dark achromatic pixels for detecting dark text on
	// any coloured background. A pixel is mapped to black (text) when its
	// BT.709 luminance ≤ darkTextMaxLum AND its channel spread (max−min) ≤
	// brightTextMaxSpread; everything else becomes white (background).
	// This is the mirror of BrightText: where BrightText finds white labels on
	// coloured buttons, DarkText finds dark labels (e.g. #333) on
	// light-coloured buttons such as the WARNING button (#f0ad4e yellow), where
	// normal grayscale preprocessing loses contrast between the dark text and the
	// medium-luminance yellow background.
	// Ignored when BrightText, ColorInverted, Color, or Inverted is true.
	DarkText bool

	// CharacterSet restricts OCR to specific characters for improved accuracy.
	// Use for specific contexts:
	// - Buttons: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	// - Numeric: "0123456789.$€£¥%"
	// - Email: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@._-"
	// Empty = all characters (default)
	CharacterSet string

	// Language specifies the OCR language pack(s) to use.
	// Default: "eng" (English)
	// Multiple languages: "eng+fra+deu" (English + French + German)
	// Requires corresponding .traineddata files in TESSDATA_PREFIX
	Language string

	// PageSegMode controls text layout analysis.
	// Default: PSM_SPARSE_TEXT (11) - finds text anywhere without layout assumptions
	// Other options:
	// - PSM_SINGLE_LINE (13) - Treat image as single text line (status bars)
	// - PSM_SINGLE_WORD (12) - Find single word (icon labels)
	// - PSM_RAW_LINE (14) - Single line, no layout analysis
	PageSegMode int
}

// MinConfidence is the minimum word confidence (0–100) to include in results.
// Words below this are OCR noise and should be discarded.
// Lowered from 50 to 35 to catch more UI elements at the cost of some noise.
const MinConfidence = 35.0

// brightTextMaxSpread is the maximum allowed channel spread (max−min across R,G,B)
// for a pixel to be classified as near-white/near-dark achromatic text in
// brightTextToGray and darkTextToGray.
//
// Value 130 is chosen to catch anti-aliased edges of white text on the full
// range of button gradient colours used in practice:
//   - White on purple #667eea (50% blend): spread=66  ≤ 130 ✓
//   - White on warning red #f5576c (50%):  spread=79  ≤ 130 ✓
//   - White on cyan #00f2fe (50% blend):   spread=127 ≤ 130 ✓  (was failing at 100)
//   - White on green #38ef7d (50% blend):  spread=92  ≤ 130 ✓
//
// For dark text, anti-aliased edges blended with a yellow background also pass:
//   - #333 on #f0ad4e (50% blend → (146,112,65)): spread=81 ≤ 130 ✓
//
// Pure coloured backgrounds are still excluded because their luminance check
// fails first (lum < 185 for bright; lum > 120 for dark) or their spread
// far exceeds 130:
//   - Pure #00f2fe (0,242,254):   lum=191 but spread=254 > 130 ✓
//   - Pure #38ef7d (56,239,125):  lum=192 but spread=183 > 130 ✓
//   - Pure #4facfe (79,172,254):  lum=158 < 185 ✓
//   - Yellow #f0ad4e (240,173,78): lum=180 > 120 (excluded by darkTextMaxLum) ✓
const brightTextMaxSpread = 130

// darkTextMaxLum is the maximum allowed BT.709 luminance for a pixel to be
// classified as near-dark text in darkTextToGray.
//
// Value 120 is chosen to catch dark text (#333, lum=51) used on warning-style
// buttons and its anti-aliased edges blended into a yellow background
// (#f0ad4e, lum=180):
//   - Pure #333 (51,51,51):            lum=51  ≤ 120 ✓
//   - 50% blend with #f0ad4e yellow:   lum=115 ≤ 120 ✓  (anti-aliased edge)
//
// Light/coloured backgrounds are excluded because their luminance exceeds 120:
//   - Yellow #f0ad4e (240,173,78): lum=180 > 120 ✓
//   - White #ffffff (255,255,255): lum=255 > 120 ✓
//   - Light gray #f5f5f5:          lum=245 > 120 ✓
const darkTextMaxLum = 120

// Common character sets for specific OCR contexts
const (
	// CharSetButtons - Alphanumeric only, best for button text
	CharSetButtons = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	// CharSetNumeric - Digits and common numeric symbols
	CharSetNumeric = "0123456789.$€£¥%+-"

	// CharSetEmail - Email address characters
	CharSetEmail = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@._-+"

	// CharSetAll - All characters (default, no restriction)
	CharSetAll = ""
)

// Page Segmentation Mode constants (from gosseract)
const (
	PSM_SPARSE_TEXT = 11 // Default - find text anywhere without layout assumptions
	PSM_SINGLE_WORD = 12 // Treat image as a single word
	PSM_SINGLE_LINE = 13 // Treat image as a single text line
	PSM_RAW_LINE    = 14 // Treat image as a single line, no layout analysis
)

var (
	cacheMu     sync.RWMutex
	cacheHash   uint64
	cacheResult *Result
)

// HashImageFast computes a fast hash over the pixel data of an image.
// Extensively optimized for *image.RGBA which is what robotgo returns.
func HashImageFast(img image.Image) uint64 {
	h := fnv.New64a()
	if rgba, ok := img.(*image.RGBA); ok {
		_, _ = h.Write(rgba.Pix)
	} else if gray, ok := img.(*image.Gray); ok {
		_, _ = h.Write(gray.Pix)
	} else {
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, bl, a := img.At(x, y).RGBA()
				_, _ = h.Write([]byte{byte(r), byte(g), byte(bl), byte(a)})
			}
		}
	}
	return h.Sum64()
}

// GetCachedResult returns the cached OCR result for the given image hash, if any.
func GetCachedResult(hash uint64) *Result {
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	if cacheHash == hash && cacheResult != nil {
		return cacheResult
	}
	return nil
}

// SetCachedResult stores the OCR result for the given image hash.
func SetCachedResult(hash uint64, res *Result) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	cacheHash = hash
	cacheResult = res
}

const pooledClientWarmCount = 4

type ocrClient interface {
	SetImage(string) error
	SetImageFromBytes([]byte) error
	SetPageSegMode(gosseract.PageSegMode) error
	SetVariable(gosseract.SettableVariable, string) error
	SetLanguage(...string) error
	GetBoundingBoxes(gosseract.PageIteratorLevel) ([]gosseract.BoundingBox, error)
	Close() error
}

type gosseractClient struct {
	*gosseract.Client
}

var (
	newOCRClient = func() ocrClient {
		return gosseractClient{Client: gosseract.NewClient()}
	}
	warmClientOnce sync.Once
	warmImageBytes = mustEncodeWarmupImage()
)

// clientPool gracefully recycles loaded Tesseract clients to eliminate the
// ~200ms `eng.traineddata` disk-initialization latency for every concurrent OCR pass.
// `sync.Pool` also relies on the natively embedded Go Garbage Collector to scale down and
// destroy idle Tesseract pointers out of RAM organically, preventing memory leaks!
var clientPool = sync.Pool{
	New: func() any {
		c := newOCRClient()
		// If we can't init, we silently return the client anyway, and let the
		// caller handle an empty or errored SetImage. However, SetPageSegMode
		// rarely fails unless Tesseract is missing completely on the OS level.
		c.SetPageSegMode(gosseract.PSM_SPARSE_TEXT)
		return c
	},
}

// getPooledClient pops a hot Tesseract client off the internal array or builds it.
func getPooledClient() ocrClient {
	primeClientPool()
	return clientPool.Get().(ocrClient)
}

// putPooledClient safely re-queues the client back into the idle pool.
// gosseract does not expose a "clear current image" API, so the previous pix
// buffer is released on the next SetImage*/Close call rather than at put time.
func putPooledClient(c ocrClient) {
	if c != nil {
		clientPool.Put(c)
	}
}

func primeClientPool() {
	warmClientOnce.Do(func() {
		warmed := make([]ocrClient, 0, pooledClientWarmCount)
		for i := 0; i < pooledClientWarmCount; i++ {
			client := clientPool.Get().(ocrClient)
			_ = warmOCRClient(client)
			warmed = append(warmed, client)
		}
		for _, client := range warmed {
			clientPool.Put(client)
		}
	})
}

func warmOCRClient(client ocrClient) error {
	if err := client.SetImageFromBytes(warmImageBytes); err != nil {
		return err
	}
	_, err := client.GetBoundingBoxes(gosseract.RIL_WORD)
	return err
}

func mustEncodeWarmupImage() []byte {
	img := image.NewGray(image.Rect(0, 0, 1, 1))
	img.SetGray(0, 0, color.Gray{Y: 255})
	buf, err := encodeForOCR(img, 1, Options{})
	if err != nil {
		panic(fmt.Sprintf("encode warmup image: %v", err))
	}
	return buf
}

// ScaleFactor is how much we upscale the image before OCR.
// Screen captures are ~96 DPI; Tesseract works best at ~300 DPI.
// 3x brings a 96 DPI capture to ~288 DPI, which sits in Tesseract's
// optimal range and measurably improves recognition of short UI text
// (button labels, menu items) compared to 2x. The extra memory cost
// (~9x pixels vs 4x) is acceptable for interactive use.
const ScaleFactor = 3

func ReadFile(imagePath string, opts Options) (*Result, error) {
	client := getPooledClient()
	defer putPooledClient(client)

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
	result, err := readClientResult(client, ScaleFactor)
	if err != nil {
		return nil, fmt.Errorf("get bounding boxes: %w. Make sure Tesseract OCR is installed: https://github.com/tesseract-ocr/tesseract", err)
	}
	return result, nil
}

// ReadImage runs OCR on an already-decoded image and returns structured output.
// It skips all file I/O — useful when the caller already has an image in memory
// (e.g. from a screen capture) and does not want to pay the cost of writing a
// temp file just to read it back.
func ReadImage(img image.Image, opts Options) (*Result, error) {
	h := HashImageFast(img)
	if cached := GetCachedResult(h); cached != nil {
		return cached, nil
	}

	prepared, err := PrepareImageSet(img, opts)
	if err != nil {
		return nil, err
	}

	res, err := ReadPreparedBytes(prepared.Normal, ScaleFactor, opts)
	if err == nil {
		SetCachedResult(h, res)
	}
	return res, err
}

// PreparedImageSet stores preprocessed OCR-ready bytes so multiple passes can
// reuse the expensive scaling work instead of each goroutine rebuilding it.
type PreparedImageSet struct {
	Normal        []byte
	Inverted      []byte
	BrightText    []byte
	DarkText      []byte
	Color         []byte
	ColorInverted []byte
}

func PrepareImageSet(img image.Image, opts Options) (*PreparedImageSet, error) {
	normal, err := encodeForOCR(img, ScaleFactor, opts)
	if err != nil {
		return nil, fmt.Errorf("preprocess image: %w", err)
	}

	return &PreparedImageSet{Normal: normal}, nil
}

func PrepareParallelImageSet(img image.Image, grayscale bool) (*PreparedImageSet, error) {
	if !grayscale {
		colorBytes, err := encodeForOCR(img, ScaleFactor, Options{Color: true})
		if err != nil {
			return nil, fmt.Errorf("preprocess color image: %w", err)
		}
		return &PreparedImageSet{Normal: colorBytes, Color: colorBytes}, nil
	}

	// Run all 6 preprocessing passes concurrently. Each pass reads img (no writes)
	// and writes to its own independent output buffer, so no synchronisation is needed
	// beyond waiting for all goroutines to finish.
	type encodeResult struct {
		name string
		data []byte
		err  error
	}
	ch := make(chan encodeResult, 6)

	passes := [6]struct {
		name string
		opts Options
	}{
		{"normal", Options{}},
		{"inverted", Options{Inverted: true}},
		{"bright-text", Options{BrightText: true}},
		{"dark-text", Options{DarkText: true}},
		{"color", Options{Color: true}},
		{"color-inverted", Options{ColorInverted: true}},
	}
	for _, p := range passes {
		p := p
		go func() {
			data, err := encodeForOCR(img, ScaleFactor, p.opts)
			ch <- encodeResult{p.name, data, err}
		}()
	}

	set := &PreparedImageSet{}
	for range passes {
		r := <-ch
		if r.err != nil {
			return nil, fmt.Errorf("preprocess %s image: %w", r.name, r.err)
		}
		switch r.name {
		case "normal":
			set.Normal = r.data
		case "inverted":
			set.Inverted = r.data
		case "bright-text":
			set.BrightText = r.data
		case "dark-text":
			set.DarkText = r.data
		case "color":
			set.Color = r.data
		case "color-inverted":
			set.ColorInverted = r.data
		}
	}
	return set, nil
}

// ReadPreparedBytes runs OCR on already-preprocessed image bytes.
func ReadPreparedBytes(imgBytes []byte, scaleFactor int, opts Options) (*Result, error) {
	client := getPooledClient()
	defer putPooledClient(client)

	if err := client.SetImageFromBytes(imgBytes); err != nil {
		return nil, fmt.Errorf("set image from bytes: %w", err)
	}

	// Apply OCR options
	applyOCROptions(client, opts)

	return readClientResult(client, scaleFactor)
}

// applyOCROptions configures Tesseract with character whitelist, language, and PSM mode
func applyOCROptions(client ocrClient, opts Options) {
	// Apply character whitelist if specified
	if opts.CharacterSet != "" {
		client.SetVariable(gosseract.TESSEDIT_CHAR_WHITELIST, opts.CharacterSet)
	}

	// Apply language if specified
	if opts.Language != "" && opts.Language != "eng" {
		client.SetLanguage(opts.Language)
	}

	// Apply page segmentation mode if different from default
	if opts.PageSegMode != 0 && opts.PageSegMode != 11 { // 11 = PSM_SPARSE_TEXT (default)
		client.SetPageSegMode(gosseract.PageSegMode(opts.PageSegMode))
	}
}

func readClientResult(client ocrClient, scaleFactor int) (*Result, error) {
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
			X:          b.Box.Min.X / scaleFactor,
			Y:          b.Box.Min.Y / scaleFactor,
			Width:      (b.Box.Max.X - b.Box.Min.X) / scaleFactor,
			Height:     (b.Box.Max.Y - b.Box.Min.Y) / scaleFactor,
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
//  1. BrightText: near-white achromatic pixels → black, else → white.
//     Isolates white button text from any coloured background. Upscaled as Gray.
//  2. DarkText: near-dark achromatic pixels → black, else → white.
//     Isolates dark button text (e.g. #333) from light-coloured backgrounds
//     such as the WARNING yellow (#f0ad4e). Upscaled as Gray.
//  3. ColorInverted: RGB-invert then grayscale. Upscaled as Gray.
//  4. Color: no grayscale/inversion; upscale as RGBA.
//  5. Default (grayscale): BT.709 grayscale + contrast stretch + optional
//     inversion (opts.Inverted). Upscaled as Gray.
func encodeForOCR(img image.Image, factor int, opts Options) ([]byte, error) {
	var scaledImg image.Image
	if opts.BrightText {
		preprocessed := brightTextToGray(img, 185)
		scaledImg = scaleGrayNearest(preprocessed, factor)
	} else if opts.DarkText {
		preprocessed := darkTextToGray(img)
		scaledImg = scaleGrayNearest(preprocessed, factor)
	} else if opts.ColorInverted {
		preprocessed := colorInvertGray(img)
		scaledImg = scaleGrayNearest(preprocessed, factor)
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
		scaledImg = scaleGrayNearest(preprocessed, factor)
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

// scaleGrayNearest performs nearest-neighbor upscaling for grayscale images
// without routing through x/image/draw's generic RGBA64 path, which creates
// tens of millions of tiny allocation objects for large *image.Gray scales.
func scaleGrayNearest(src *image.Gray, factor int) *image.Gray {
	b := src.Bounds()
	srcW, srcH := b.Dx(), b.Dy()

	if factor <= 1 {
		clone := image.NewGray(image.Rect(0, 0, srcW, srcH))
		for y := 0; y < srcH; y++ {
			srcOff := y * src.Stride
			dstOff := y * clone.Stride
			copy(clone.Pix[dstOff:dstOff+srcW], src.Pix[srcOff:srcOff+srcW])
		}
		return clone
	}

	dst := image.NewGray(image.Rect(0, 0, srcW*factor, srcH*factor))

	for sy := 0; sy < srcH; sy++ {
		srcRow := src.Pix[sy*src.Stride : sy*src.Stride+srcW]
		for fy := 0; fy < factor; fy++ {
			dstRow := dst.Pix[(sy*factor+fy)*dst.Stride : (sy*factor+fy)*dst.Stride+srcW*factor]
			for sx, px := range srcRow {
				base := sx * factor
				for fx := 0; fx < factor; fx++ {
					dstRow[base+fx] = px
				}
			}
		}
	}

	return dst
}

// invertGray flips every pixel in-place: new = 255 - old.
func invertGray(img *image.Gray) {
	for i, v := range img.Pix {
		img.Pix[i] = 255 - v
	}
}

// colorInvertGray inverts the RGB image (255−R, 255−G, 255−B) BEFORE
// converting to grayscale. This produces high-contrast dark text on a
// medium-gray background, which is what Tesseract is trained on.
//
// Example — white text on cyan #4facfe:
//
//	Normal grayscale:     white→255, cyan→158  (gap 97, low contrast)
//	Inverted (after gray): white→0,   cyan→97   (gap 97, dark-on-darker)
//	ColorInverted:        white→0,   cyan→83   (gap 83, black-on-medium)
//
// The critical difference: regular Inverted keeps the same luminance gap but
// shifts it to the dark end of the scale (0→97), while ColorInvert produces
// black text (0) on a medium-gray background that Tesseract can reliably read.
func colorInvertGray(img image.Image) *image.Gray {
	b := img.Bounds()
	out := image.NewGray(b)
	w, h := b.Dx(), b.Dy()

	// Fast path for *image.RGBA
	if rgba, ok := img.(*image.RGBA); ok {
		for y := 0; y < h; y++ {
			srcRow := rgba.Pix[y*rgba.Stride : y*rgba.Stride+w*4]
			dstOff := y * out.Stride
			for x := 0; x < w; x++ {
				r := 255 - srcRow[x*4]
				g := 255 - srcRow[x*4+1]
				bl := 255 - srcRow[x*4+2]
				// BT.709 luminance, scaled ×10000
				lum := (2126*uint32(r) + 7152*uint32(g) + 722*uint32(bl) + 5000) / 10000
				out.Pix[dstOff+x] = uint8(lum)
			}
		}
	} else {
		// Slow path: interface dispatch
		for y := b.Min.Y; y < b.Max.Y; y++ {
			dstOff := (y-b.Min.Y)*out.Stride - b.Min.X
			for x := b.Min.X; x < b.Max.X; x++ {
				r32, g32, b32, _ := img.At(x, y).RGBA()
				r := 255 - uint8(r32>>8)
				g := 255 - uint8(g32>>8)
				bl := 255 - uint8(b32>>8)
				lum := (2126*uint32(r) + 7152*uint32(g) + 722*uint32(bl) + 5000) / 10000
				out.Pix[dstOff+x] = uint8(lum)
			}
		}
	}
	return out
}

// brightTextToGray creates a binary image where near-white pixels become black
// (text) and everything else becomes white (background). It is designed to
// isolate white button labels — including their anti-aliased edges — from
// any coloured or dark background.
//
// Detection condition (both must hold):
//
//  1. BT.709 luminance ≥ threshold  (pixel is bright enough to be white text
//     or a lightly blended edge).
//  2. max(R,G,B) − min(R,G,B) ≤ brightTextMaxSpread  (pixel is achromatic
//     enough — high-luma coloured backgrounds like cyan #00f2fe or bright green
//     #38ef7d have spread > 200 and are excluded here even though their
//     luminance is high).
//
// Why luminance instead of min(R,G,B):
// Gradients like the WARNING button (#f093fb→#f5576c) have a very low green
// channel on the red end (G=87). A 50%-blended anti-aliased edge produces
// (250,171,181), where min=171 — acceptable — but a 60% blend gives min=154,
// which falls below any min-channel threshold that also excludes the pink start
// (#f093fb, min=147). Luminance side-steps this: lum(250,171,181)=188.5 ≥ 185
// while lum(240,147,251)=174.3 < 185, giving a clean 14-point gap.
//
// The production threshold is 185 (set by the caller in encodeForOCR).
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
				// BT.709 luminance, scaled ×10000 to stay integer.
				lum := (2126*uint32(r) + 7152*uint32(g) + 722*uint32(bl) + 5000) / 10000
				// Channel spread: high spread = colourful (not near-white).
				hi, lo := r, r
				if g > hi {
					hi = g
				} else if g < lo {
					lo = g
				}
				if bl > hi {
					hi = bl
				} else if bl < lo {
					lo = bl
				}
				if lum >= uint32(threshold) && hi-lo <= brightTextMaxSpread {
					out.Pix[dstOff+x] = 0 // near-white, low-saturation → black (text)
				} else {
					out.Pix[dstOff+x] = 255 // coloured or dark → white (background)
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
				lum := (2126*uint32(r8) + 7152*uint32(g8) + 722*uint32(b8) + 5000) / 10000
				hi, lo := r8, r8
				if g8 > hi {
					hi = g8
				} else if g8 < lo {
					lo = g8
				}
				if b8 > hi {
					hi = b8
				} else if b8 < lo {
					lo = b8
				}
				if lum >= uint32(threshold) && hi-lo <= brightTextMaxSpread {
					out.Pix[dstOff+x] = 0
				} else {
					out.Pix[dstOff+x] = 255
				}
			}
		}
	}
	return out
}

// darkTextToGray creates a binary image where near-dark achromatic pixels become
// black (text) and everything else becomes white (background). It is the mirror
// of brightTextToGray, designed to isolate dark button labels — and their
// anti-aliased edges — from any coloured background.
//
// Detection condition (both must hold):
//
//  1. BT.709 luminance ≤ darkTextMaxLum  (pixel is dark enough to be dark text
//     or a lightly-blended anti-aliased edge against a light background).
//  2. max(R,G,B) − min(R,G,B) ≤ brightTextMaxSpread  (pixel is achromatic
//     enough — coloured backgrounds like yellow #f0ad4e have spread > 160 and
//     are excluded even when their luminance is relatively low).
//
// Primary use case: WARNING-style buttons with dark (#333) text on a
// yellow/orange background (#f0ad4e) where grayscale preprocessing loses the
// contrast between the dark text (lum=51) and the medium-luminance yellow
// background (lum=180).
func darkTextToGray(img image.Image) *image.Gray {
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
				// BT.709 luminance, scaled ×10000 to stay integer.
				lum := (2126*uint32(r) + 7152*uint32(g) + 722*uint32(bl) + 5000) / 10000
				// Channel spread: high spread = colourful (not near-dark text).
				hi, lo := r, r
				if g > hi {
					hi = g
				} else if g < lo {
					lo = g
				}
				if bl > hi {
					hi = bl
				} else if bl < lo {
					lo = bl
				}
				if lum <= darkTextMaxLum && hi-lo <= brightTextMaxSpread {
					out.Pix[dstOff+x] = 0 // near-dark, low-saturation → black (text)
				} else {
					out.Pix[dstOff+x] = 255 // coloured or bright → white (background)
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
				lum := (2126*uint32(r8) + 7152*uint32(g8) + 722*uint32(b8) + 5000) / 10000
				hi, lo := r8, r8
				if g8 > hi {
					hi = g8
				} else if g8 < lo {
					lo = g8
				}
				if b8 > hi {
					hi = b8
				} else if b8 < lo {
					lo = b8
				}
				if lum <= darkTextMaxLum && hi-lo <= brightTextMaxSpread {
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
