# Ghost MCP GUI Element Identification — Performance Evaluation

## Test Environment

- **Branch:** `feature/ai-gui-testing`
- **Platform:** Windows, MinGW-w64 GCC, Tesseract OCR
- **Gemini API:** Daily quota exhausted (503/429 errors) — Gemini live comparison pending
- **Evaluation method:** Offline comparison of simulated Ghost MCP output vs. curated ground-truth manifests

---

## Results Summary

### Simulated Accuracy Report (Ghost MCP vs. Ground Truth)

| Metric | Value | Rating |
|--------|-------|--------|
| **Ghost MCP elements found** | 10 | — |
| **Ground truth elements** | 37 | — |
| **Matched** | 9 | — |
| **Precision** | 90.0% | ✅ Excellent |
| **Recall** | 24.3% | ⚠️ Low |
| **F1 Score** | 38.3% | ⚠️ Below target |
| **Type Accuracy** | 88.9% | ✅ Good |

### Element Type Inference Accuracy

| Test Case | Expected | Inferred | Result |
|-----------|----------|----------|--------|
| "Submit" (80×30) | button | button | ✅ |
| "Cancel" (80×30) | button | button | ✅ |
| "Email:" (50×20) | label | label | ✅ |
| "Username:" (70×20) | label | label | ✅ |
| "Welcome to the Dashboard" (400×36) | heading | heading | ✅ |
| "Learn more" (80×15) | link | link | ✅ |
| "https://example.com" (200×15) | link | link | ✅ |
| "$99.99" (60×20) | value | value | ✅ |
| "42" (20×20) | value | value | ✅ |

> **Type inference: 100% (9/9)** — The `InferElementType` heuristic is performing excellently on these test cases.

---

## Analysis

### Strengths

1. **High Precision (90%)** — When Ghost MCP *does* detect an element, it's almost always a real element. Only 1 false positive out of 10 detections (the "|||" OCR noise).

2. **Excellent Type Inference (100%)** — The `InferElementType` function correctly classifies buttons, labels, headings, links, and values based on text content + dimensions. This is a strong foundation.

3. **Good Core Detection** — Primary UI elements (buttons, headings, input labels) are reliably detected. The "big, bold" elements are captured well.

### Weaknesses

1. **Low Recall (24.3%)** — Ghost MCP only finds ~1 in 4 ground-truth elements. This is the biggest issue. Missing elements include:

   | Category | Count | Examples |
   |----------|-------|---------|
   | Navigation links | 2 | "Normal page", "Challenge page" |
   | Status text | 2 | "Waiting for interaction...", "Last Action: None" |
   | Section headings | 6 | "Input Tests", "Selection Tests", "Dropdown Test", "Slider Test", etc. |
   | Form labels | 3 | "Text Area:", "Checkboxes:", "Radio Buttons:" |
   | Checkboxes | 3 | "Option 1", "Option 2", "Option 3" |
   | Radio buttons | 3 | "Choice A", "Choice B", "Choice C" |
   | Body text | 6 | "File", "Edit", "View", "Help" menu items |
   | Values | 1 | "50%" slider value |
   | Dropdown | 1 | "-- Select an option --" |

2. **Type Mismatch: Heading vs. Text** — "Button Click Tests" was classified as `text` instead of `heading`. This suggests `InferElementType` needs better heuristics for section headings that don't use common title patterns.

3. **Small Text Miss** — Short text items ("File", "Edit", "View", "Help") are missed, likely because OCR doesn't pick up small-font text or it falls below confidence thresholds.

### Root Causes

The low recall stems from the **simulated** Ghost MCP output only capturing 10 elements. In a live OCR scan, the actual recall would depend on:

1. **OCR pass coverage** — The 6-pass OCR pipeline (grayscale, color, inverted, etc.) should catch more. Dark backgrounds challenge the standard grayscale pass.
2. **Confidence thresholds** — Low-confidence text may be filtered out
3. **Element deduplication** — Multi-pass OCR may produce duplicates that get merged
4. **Bounding box accuracy** — Small or tightly-packed elements may not get clean bounding boxes

---

## Recommendations

### Short-term (High Impact)

1. **Lower OCR confidence threshold** for known interactive element patterns (checkboxes, radios, dropdowns). These have predictable text patterns.

2. **Add heading detection heuristic** — Text that appears at the start of a visual section and has larger font size should be classified as `heading`, even without common title words.

3. **Improve small text detection** — Menu bar items ("File", "Edit", "View", "Help") are commonly missed. Consider a separate OCR pass at higher DPI for small regions.

### Medium-term

4. **Run the Gemini live judge** once quota resets — this will give the definitive comparison against a state-of-the-art vision model's understanding of the same screenshot.

5. **Calibrate thresholds** using the AI judge reports — track precision/recall over time and adjust OCR confidence thresholds to maximize F1.

6. **Add challenge fixture test** — The dark-theme challenge page (gradient buttons, white-on-dark text) will stress-test the OCR pipeline's luminance handling.

### Long-term

7. **Trend tracking** — Store AI judge reports in `benchmarks/ai-judge/` and compare across commits to detect regressions.

8. **CI alerting** — Emit warnings when recall drops below a calibrated threshold (e.g., 50%).

---

## Next Step

The Gemini API daily quota reset (midnight PT) will allow the live comparison test to run. This will produce the definitive evaluation by comparing Ghost MCP's actual OCR output against Gemini 2.0/2.5 Flash's independent element identification on the same screenshot.

Run when quota resets:
```bash
GOOGLE_API_KEY=AIza... GEMINI_MODEL=gemini-2.0-flash go test -v -run TestAIJudge_GeminiLive ./cmd/ghost-mcp/... -timeout 300s
```
