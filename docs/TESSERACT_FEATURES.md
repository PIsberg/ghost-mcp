# Tesseract Features for UI Automation

## Current Usage

### ✅ Already Implemented
1. **PSM_SPARSE_TEXT** - Page segmentation mode for scattered UI text
2. **Multi-pass OCR** - Normal, Inverted, BrightText, Color passes
3. **Confidence filtering** - MinConfidence = 35
4. **Image caching** - Avoids redundant OCR on same images
5. **Element type inference** - Classifies buttons, labels, inputs, etc.
6. **Text variation fallback** - Tries variations when exact match fails

---

## Additional Tesseract Features Worth Adding

### 1. **Character Whitelist/Blacklist** 🎯 HIGH VALUE

**What it does:** Restricts OCR to specific character sets

**Use case:** 
- Button text: Only alphanumeric + common punctuation
- Numeric fields: Only digits, decimals, currency symbols
- Email fields: Alphanumeric + @ + .

**Implementation:**
```go
// In Options struct
CharacterSet string  // e.g., "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// In ReadImage()
if opts.CharacterSet != "" {
    client.SetVariable("tessedit_char_whitelist", opts.CharacterSet)
}
```

**Benefit:** 
- Reduces false positives (e.g., "0" vs "O", "1" vs "l")
- Improves accuracy for specific contexts
- Faster recognition (fewer candidates to check)

**Example usage:**
```go
// For numeric-only fields (prices, counters)
ocr.Options{CharacterSet: "0123456789.$€£¥%"}

// For button text (no special chars)
ocr.Options{CharacterSet: "a-zA-Z0-9"}
```

---

### 2. **Multiple PSM Modes** 🎯 MEDIUM VALUE

**What it does:** Different page segmentation strategies

**Current:** PSM_SPARSE_TEXT (11) - finds text anywhere

**Additional modes to try:**
- **PSM_SINGLE_LINE** (13) - Treat image as single text line
  - Good for: Status bars, single-line inputs
- **PSM_SINGLE_WORD** (12) - Find single word
  - Good for: Icon labels, short buttons
- **PSM_RAW_LINE** (14) - Treat as single line, no layout analysis
  - Good for: Dense text areas

**Implementation:**
```go
// Try multiple PSM modes in parallel
psmModes := []gosseract.PageSegMode{
    gosseract.PSM_SPARSE_TEXT,  // Default
    gosseract.PSM_SINGLE_LINE,  // For status bars
    gosseract.PSM_SINGLE_WORD,  // For short labels
}

// Use best result
```

**Benefit:**
- Better accuracy for specific UI patterns
- Reduces missed detections in edge cases

---

### 3. **Language Packs** 🎯 MEDIUM VALUE

**What it does:** Support for multiple languages

**Current:** English only (eng.traineddata)

**Additional languages:**
- European: fra, deu, spa, ita, por
- Asian: chi_sim, chi_tra, jpn, kor
- RTL: ara, heb

**Implementation:**
```go
// In Options struct
Language string  // Default: "eng"

// In ReadImage()
if opts.Language != "" {
    client.SetLanguage(opts.Language)
}

// Multi-language support
client.SetLanguage("eng+fra+deu")  // English + French + German
```

**Benefit:**
- International app support
- Better accuracy for non-English UIs
- Mixed-language UIs (e.g., English UI with Spanish content)

**Note:** Requires installing additional tessdata files

---

### 4. **Orientation Detection (OSD)** 🎯 LOW VALUE

**What it does:** Detects text orientation (0°, 90°, 180°, 270°)

**Use case:**
- Rotated screens (mobile emulators, tablets)
- Mirrored displays
- Apps with rotated UI elements

**Implementation:**
```go
// Before OCR, detect orientation
osdResult := client.GetOSD()
if osdResult.Orientation > 0 {
    // Rotate image before OCR
    img = rotateImage(img, osdResult.Orientation)
}
```

**Benefit:**
- Handles rotated/mirrored screens
- Useful for mobile automation

**Drawback:**
- Adds ~100-200ms overhead
- Rarely needed for desktop apps

---

### 5. **Line-Level vs Word-Level Detection** 🎯 HIGH VALUE

**What it does:** Get bounding boxes at different granularities

**Current:** Word-level only (each word separately)

**Additional levels:**
- **Line-level** - Entire line as one unit
  - Better for: Labels, multi-word buttons
  - Preserves word order and spacing
- **Paragraph-level** - Block of text
  - Better for: Text areas, descriptions

**Implementation:**
```go
// Get both word and line results
words := client.GetBoundingBoxes(gosseract.WORD)
lines := client.GetBoundingBoxes(gosseract.TEXTLINE)

// Merge words into lines for better context
elements := mergeWordsIntoLines(words, lines)
```

**Benefit:**
- Better element type inference (full context)
- Preserves multi-word button labels
- Reduces fragmentation ("Click" + "Here" → "Click Here")

---

### 6. **LSTM vs Legacy Engine Mode** 🎯 LOW VALUE

**What it does:** Choose between neural network (LSTM) and legacy Tesseract

**Current:** LSTM (default, best for most cases)

**Legacy mode:**
- Better for: Unusual fonts, decorative text
- Worse for: Standard UI fonts

**Implementation:**
```go
// In Options struct
EngineMode int  // 0=LSTM, 1=Legacy, 2=Both, 3=Default

// In ReadImage()
client.SetVariable("tessedit_ocr_engine_mode", fmt.Sprintf("%d", opts.EngineMode))
```

**Benefit:**
- Fallback for decorative fonts
- Better accuracy for specific edge cases

**Drawback:**
- Legacy is slower and less accurate for standard fonts
- Adds complexity

---

### 7. **Custom Dictionary/User Patterns** 🎯 MEDIUM VALUE

**What it does:** Provide custom word lists for better recognition

**Use case:**
- App-specific terminology
- Brand names, product names
- Technical jargon

**Implementation:**
```go
// Create user patterns file
userPatterns := []string{
    "Ghost MCP",
    "learn_screen",
    "find_and_click",
    "TypeScript",
    "React",
}

// In ReadImage()
client.SetVariable("user_patterns_file", "/path/to/patterns.txt")
```

**Benefit:**
- Better accuracy for app-specific terms
- Reduces misrecognition of technical terms

---

### 8. **Text Orientation Variables** 🎯 LOW VALUE

**What it does:** Force specific text orientation

**Variables:**
- `textord_excess_blobsize` - Adjust for dense text
- `classify_bln_numeric_group` - Better number recognition
- `tessedit_do_invert` - Additional inversion passes

**Implementation:**
```go
// For numeric-heavy UIs
client.SetVariable("classify_bln_numeric_group", "1")

// For dense UIs
client.SetVariable("textord_excess_blobsize", "150")
```

**Benefit:**
- Fine-tuned for specific UI types
- Marginal accuracy improvements

---

## Recommended Priority

### 🚀 Immediate (High Value, Low Effort)

1. **Character Whitelist** - Add to `find_and_click` for button text
2. **Line-Level Detection** - Merge words into lines for better context

### 📅 Short-term (Medium Value)

3. **Multiple PSM Modes** - Try SINGLE_LINE for status bars
4. **Custom Patterns** - Add app-specific terms
5. **Language Support** - Add common European languages

### 🔮 Future (Low Value / Specialized)

6. **Orientation Detection** - Only if mobile/tablet support needed
7. **Legacy Engine** - Only if decorative fonts cause issues
8. **Text Orientation Variables** - Fine-tuning only

---

## Implementation Example: Character Whitelist

```go
// In internal/ocr/ocr.go

type Options struct {
    // ... existing fields ...
    
    // CharacterSet restricts OCR to specific characters.
    // Use for improved accuracy in specific contexts:
    // - Numeric: "0123456789.$€£¥%"
    // - Buttons: "a-zA-Z0-9"
    // - Email: "a-zA-Z0-9@._-"
    // Empty = all characters (default)
    CharacterSet string
}

// In ReadImage()
func ReadImage(img image.Image, opts Options) (*Result, error) {
    // ... existing preprocessing ...
    
    client := gosseract.NewClient()
    defer client.Close()
    
    client.SetPageSegMode(gosseract.PSM_SPARSE_TEXT)
    
    // Apply character whitelist if specified
    if opts.CharacterSet != "" {
        client.SetVariable("tessedit_char_whitelist", opts.CharacterSet)
    }
    
    // ... rest of OCR ...
}

// Usage in handler_ocr.go
func handleFindAndClick(...) {
    // For button text (alphanumeric only)
    ocrResult, _ := ocr.ReadImage(img, ocr.Options{
        CharacterSet: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789",
    })
}
```

---

## Performance Impact

| Feature | Overhead | Benefit |
|---------|----------|---------|
| Character whitelist | None | +5-10% accuracy |
| Line-level detection | +50ms | +15% context accuracy |
| Multiple PSM modes | +100-200ms | +10% edge cases |
| Language packs | None | Essential for i18n |
| Orientation detection | +100-200ms | Only for rotated screens |
| Legacy engine | +50% slower | Only for decorative fonts |
| Custom patterns | None | +5% for specific terms |

---

## Conclusion

**Most impactful additions:**
1. ✅ Character whitelist - Easy win for button/numeric detection
2. ✅ Line-level detection - Better context for multi-word elements
3. ✅ Language support - Essential for international apps

**Nice to have:**
- Multiple PSM modes for edge cases
- Custom patterns for app-specific terms

**Skip unless needed:**
- Orientation detection (unless mobile support)
- Legacy engine (LSTM is better for UI fonts)
- Fine-tuning variables (marginal gains)
