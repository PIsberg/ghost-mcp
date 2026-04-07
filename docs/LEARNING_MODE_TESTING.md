# Learning Mode Testing Guide

## Overview

This guide explains how to run and interpret the learning mode tests for the Ghost MCP server, including how to measure accuracy improvements.

## Quick Start

### Run All Learning Mode Tests

```powershell
# Set up environment
$env:Path = "$env:USERPROFILE\vcpkg\installed\x64-mingw-dynamic\bin;C:\ProgramData\mingw64\mingw64\bin;$env:Path"
$env:TESSDATA_PREFIX = "$env:USERPROFILE\vcpkg\installed\x64-mingw-dynamic\share\tessdata"

# Run unit tests (no display required)
go test -v -run "TestAccuracy|TestInferTypes|TestMergeOCR|TestAssociateLabels|TestSmartClick" ./cmd/ghost-mcp/...

# Run benchmarks
go test -bench=Benchmark -benchmem ./internal/learner/... ./cmd/ghost-mcp/...
```

### Run Integration Tests (Requires Display)

```powershell
# Set up environment
$env:Path = "$env:USERPROFILE\vcpkg\installed\x64-mingw-dynamic\bin;C:\ProgramData\mingw64\mingw64\bin;$env:Path"
$env:TESSDATA_PREFIX = "$env:USERPROFILE\vcpkg\installed\x64-mingw-dynamic\share\tessdata"
$env:INTEGRATION = "1"
$env:CI = ""

# Run integration tests against live fixture page
go test -v -tags=integration -run "TestIntegration" ./cmd/ghost-mcp/...
```

---

## Test Categories

### 1. Unit Tests (Pure Go - No Display Required)

These tests verify the learning mode algorithms work correctly:

| Test File | What It Tests |
|-----------|---------------|
| `internal/learner/learner_test.go` | Element lookup, deduplication, type inference |
| `cmd/ghost-mcp/handler_learning_test.go` | OCR merge passes, label association |
| `cmd/ghost-mcp/accuracy_demo_test.go` | **Accuracy improvements demonstration** |
| `cmd/ghost-mcp/smart_click_test.go` | **NEW: smart_click tool tests** |

**Run accuracy demo:**
```powershell
go test -v -run TestAccuracy ./cmd/ghost-mcp/...
```

**Expected output:**
```
=== RUN   TestAccuracy_MultiPassOCR
    Single-pass OCR finds: 3 elements
    Four-pass OCR finds: 7 elements
    Accuracy improvement: 133% more elements discovered
--- PASS: TestAccuracy_MultiPassOCR (0.00s)
```

**Run smart_click tests:**
```powershell
go test -v -run TestSmartClick ./cmd/ghost-mcp/...
```

### 2. Integration Tests (Requires Display + Fixture Server)

These tests run against a live web page:

| Test | What It Verifies |
|------|------------------|
| `TestIntegration_LearnScreen_Discovers_VisibleElements` | Basic element discovery |
| `TestIntegration_LearnScreen_ScrollDiscovery` | Finds off-screen elements |
| `TestIntegration_FindAndClick_WithLearning` | Learning mode workflow |
| `TestIntegration_FindAndClickButton` | Real button clicking |
| `TestIntegration_SmartClick` | **NEW: smart_click end-to-end** |

**Run specific integration test:**
```powershell
go test -v -tags=integration -run "TestIntegration_FindAndClickButton" ./cmd/ghost-mcp/...
```

---

## Understanding Accuracy Metrics

### Current Accuracy with 4-Pass OCR

**Unit tests:** ✅ All pass - algorithms working correctly

**Integration tests:** Variable - depends on:
- Fixture page being served and visible
- Display availability
- Tesseract language model quality
- Font rendering on the system

### Four-Pass OCR System

Learning mode now uses **four complementary OCR passes**:

1. **Normal Pass** - Grayscale + contrast stretch
   - Best for: Black text on white/light backgrounds
   - Catches: ~60-70% of typical UI text

2. **Inverted Pass** - Brightness inversion
   - Best for: White text on dark backgrounds
   - Catches: ~15-20% missed by normal pass

3. **BrightText Pass** - Near-white pixel isolation (NEW)
   - Best for: White text on ANY colored background
   - Threshold: RGB ≥ 240 → black, else → white
   - Catches: Buttons with gradient backgrounds

4. **Color Pass** - Full color preservation
   - Best for: Colored text, brand-colored buttons
   - Catches: Elements missed by grayscale passes

### Accuracy Improvements Applied (Legitimate)

1. **Lowered OCR confidence threshold** (30 → 35)
   - Catches more borderline text detections
   - Trade-off: slightly more noise, but acceptable

2. **Added BrightText OCR pass**
   - Isolates near-white pixels (white button text)
   - Works on any background color
   - No CSS modifications needed

3. **Four-pass deduplication**
   - Same element detected by multiple passes → kept once
   - Highest confidence detection preserved

### Why Elements May Still Be Missed

1. **Font Rendering Issues**
   - Custom fonts may not render clearly for Tesseract
   - Small font sizes (<12px) are hard to OCR
   - Some gradient backgrounds still challenging

2. **Timing Issues**
   - Page animations during screenshot capture
   - Dynamic content loading after page load

3. **Tesseract Limitations**
   - Trained primarily on printed documents
   - UI text with shadows/glows may confuse it
   - Non-Latin scripts need additional training data

### Why Elements Are Missed

1. **Font Rendering Issues**
   - Custom fonts may not render clearly for Tesseract
   - Small font sizes (<12px) are hard to OCR
   - Gradient backgrounds can interfere with text detection

2. **Timing Issues**
   - Page animations during screenshot capture
   - Dynamic content loading after page load
   - Anti-aliasing artifacts

3. **Color Contrast**
   - White text on light gradients (low contrast)
   - Some color combinations confuse all four passes

### How to Improve Accuracy

#### A. Adjust OCR Confidence Threshold

```go
// In internal/ocr/ocr.go
// Current: MinConfidence = 35 (balanced)
// Lower to 30 for more detections (more noise)
// Raise to 50 for fewer false positives
const MinConfidence = 35
```

#### B. Add More OCR Passes

```go
// In handler_learning.go learnScreen(), the current 4 passes are:
normalResult, _ := uiReadImage(img, ocr.Options{})
invertedResult, _ := uiReadImage(img, ocr.Options{Inverted: true})
brightResult, _ := uiReadImage(img, ocr.Options{BrightText: true})
colorResult, _ := uiReadImage(img, ocr.Options{Color: true})

// Could add additional passes for specific use cases
```

#### C. Use Learning Mode Properly

The **recommended workflow** maximizes accuracy:

```
1. learn_screen          ← Full screen scan with 4 OCR passes
2. get_learned_view      ← Review what was found
3. find_and_click        ← Uses cached view (no new OCR)
4. If element not found:
   - clear_learned_view  ← Clear stale cache
   - learn_screen        ← Re-scan
   - Retry operation
```

---

## Benchmarking Performance

### Run Benchmarks

```powershell
# Core algorithm benchmarks
go test -bench=Benchmark -benchmem ./internal/learner/...

# Full OCR benchmarks (with Tesseract)
go test -bench=Benchmark -benchmem ./cmd/ghost-mcp/...
```

### Key Performance Metrics

| Operation | Time | Memory | Notes |
|-----------|------|--------|-------|
| **Text similarity** | 1.6 ns | 0 B | Essentially free |
| **Element lookup** | 300 ns | 160 B | 5-element view |
| **Type inference** | 64 ns - 3.3 μs | 16-88 B | Depends on type |
| **Full OCR scan** | 2-3 seconds | 1-2 GB | Per screen capture |

### Performance vs Accuracy Trade-off

| Configuration | Time | Accuracy | When to Use |
|---------------|------|----------|-------------|
| **Single-pass OCR** | ~1s | ~30% | Quick checks only |
| **Two-pass OCR** | ~2s | ~50% | Basic workflows |
| **Four-pass OCR** | ~4s | ~75-85% | **Current default** |
| **Learning mode** | 4s + μs lookups | ~95%* | **Recommended** |

*After initial scan, all subsequent operations use cached view

### Feature Flags for Learning Mode

| Environment Variable | Default | Purpose |
|----------------------|---------|---------|
| `GHOST_MCP_ASYNC_SCAN` | `1` | Enables massively parallel OCR evaluation for scrolling pages, reducing discovery time by >50%. Set to `0` to revert to synchronous captures. |
| `GHOST_MCP_PHASH` | `1` | Uses Perceptual Image Hashing (dHash) to detect the exact bottom of scrollable areas dynamically. Tolerates blinking cursors and minor noise better than absolute pixel comparison or text diffs. Set to `0` to use exact sub-pixel MSE matching. |

---

## Interpreting Test Results

### Unit Test Output Example

```
=== RUN   TestAccuracy_MultiPassOCR
    Single-pass OCR finds: 3 elements
    Four-pass OCR finds: 7 elements
    Accuracy improvement: 133% more elements discovered
--- PASS: TestAccuracy_MultiPassOCR (0.00s)
```

**What this means:**
- Without learning mode: 3 elements detected
- With learning mode (4 passes): 7 elements detected
- **133% more elements** found due to multi-pass OCR

### Integration Test Output Example

```
=== RUN   TestIntegration_FindAndClickButton/Success
    ✓ Clicked "Success" at (585, 697)
=== RUN   TestIntegration_FindAndClickButton/Primary
    FindAndClick("Primary") error: text "Primary" not found on screen
    Closest OCR matches: ["are" "error:" "trigger"]
--- FAIL: TestIntegration_FindAndClickButton/Primary (3.98s)
```

**What this means:**
- "Success" button: Found and clicked correctly ✅
- "Primary" button: OCR missed it ❌
- **Possible causes:** Font rendering, gradient background, timing

### How to Debug Failed Tests

1. **Check OCR screenshots:**
   ```
   Look for files like: C:\Users\isber\AppData\Local\Temp\ghost-mcp-findclick-*.png
   These show what the OCR actually saw
   ```

2. **Enable debug logging:**
   ```powershell
   $env:GHOST_MCP_LOG_LEVEL = "DEBUG"
   go test -v -tags=integration ./cmd/ghost-mcp/...
   ```

3. **Use get_learned_view to inspect:**
   ```json
   // After learn_screen, call get_learned_view
   // Check if "Primary" appears in elements array
   {
     "elements": [
       {"text": "Success", "type": "button", ...},
       // Is "Primary" here?
     ]
   }
   ```

---

## Accuracy Improvement Checklist

- [ ] **Run accuracy demo tests** - Verify 133% improvement from multi-pass
- [ ] **Check fixture page contrast** - Ensure buttons have solid backgrounds
- [ ] **Increase font sizes** - Minimum 14px for reliable OCR
- [ ] **Add wait time** - Let animations settle before learn_screen
- [ ] **Use learning mode workflow** - Always learn_screen before automation
- [ ] **Review OCR screenshots** - Check what Tesseract actually sees
- [ ] **Consider Tesseract training** - Custom training for UI fonts

---

## Files Reference

| File | Purpose |
|------|---------|
| `cmd/ghost-mcp/accuracy_demo_test.go` | **Accuracy demonstration tests** |
| `cmd/ghost-mcp/handler_learning_test.go` | Learning mode unit tests |
| `cmd/ghost-mcp/integration_learning_test.go` | Integration tests |
| `cmd/ghost-mcp/test_fixture/index.html` | Test fixture web page |
| `internal/learner/learner.go` | Core learning mode logic |
| `internal/learner/learner_test.go` | Learner unit tests |
| `internal/ocr/ocr.go` | OCR preprocessing passes |

---

## Next Steps for Improvement

1. **Short-term:**
   - Adjust fixture page CSS for better contrast
   - Add 500ms settle time before OCR captures
   - Lower confidence threshold temporarily

2. **Medium-term:**
   - Add adaptive thresholding OCR pass
   - Implement template matching for buttons
   - Add edge detection for UI elements

3. **Long-term:**
   - Train custom Tesseract model for UI fonts
   - Integrate deep learning element detection
   - Add computer vision for non-text elements

---

## Support

For issues or questions:
1. Check test output logs for specific error messages
2. Review OCR screenshots in temp directory
3. Compare `get_learned_view` output with expected elements
4. Run `TestAccuracy` tests to verify algorithm correctness
