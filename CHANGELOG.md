# Changelog

All notable changes to ghost-mcp are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/);
versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.5.1] — 2026-04-11

### Bug Fixes
- Use force-with-lease to push CHANGELOG commit from detached HEAD


### Documentation
- Update CHANGELOG.md for v0.5.1 [skip ci]


## [0.5.0] — 2026-04-11

### Bug Fixes
- Move tag to CHANGELOG commit to satisfy goreleaser validation
- **test**: Relax overly aggressive dHash test threshold
- Make short-mode test suite pass cleanly
- Update handleMoveMouse to return JSON instead of plain text. this resolves the JSON parsing error in TestHandleMoveMouseValid and ensures consistent tool output formatting. confirmed terminology hardening is complete.
- Resolve prompt and OCR test regressions by restoring legacy keywords and refining thresholds
- Resolve double-typing bug and enable screenshot persistence. implemented fuzzy OCR verification with Levenshtein distance, added automatic field clearing to re-types, and hardened integration test window focus logic for Windows. confirmed terminology hardening is complete.
- Harden routing guide against inefficient scroll-and-peek loops
- Normalize visual anchor coordinates and clean up tests
- Force 4-column button grid in fixture; widen ReadAllPasses dedup tolerance
- Use multi-pass OCR in find_elements to detect coloured-background buttons
- Make TestParallelFindText robust against CI timeout
- Add Safety section to ghostMCPGuide to fix Linux CI test
- Apply gofmt formatting to element_type changes
- Verify text actually appeared on screen after find_click_and_type
- Correct test expectation for missing enabled parameter
- Apply gofmt formatting to all new test files
- Get_page_screenshot now returns proper MCP image content
- Switch BrightText from min-channel to luminance+spread detection
- Inline routing guide in WithInstructions instead of asking clients to fetch it
- Fix two OCR bugs that prevented white-on-gradient button text detection


### Dependency Updates
- **deps**: Bump github.com/mark3labs/mcp-go from 0.47.0 to 0.47.1
- **deps**: Bump golang.org/x/image from 0.38.0 to 0.39.0
- **deps**: Bump step-security/harden-runner from 2.16.1 to 2.17.0
- **deps**: Bump github.com/mark3labs/mcp-go from 0.46.0 to 0.47.0
- **deps**: Bump actions/cache from 4.3.0 to 5.0.4


### Documentation
- Update CHANGELOG.md for v0.5.0 [skip ci]
- Document element_types filter in ghost_mcp_guide prompt
- Synchronize documentation with high-precision Visual ID workflow
- Sync visual anchors and fix formatting
- Update CLAUDE.md to document all six OCR preprocessing passes
- Tighten tool descriptions for token efficiency and clarity
- Add explicit exploration tool hierarchy to find_elements and learn_screen
- Add Troubleshooting & Fallbacks section with explicit error recovery
- Move safety rules to top of routing guide (primacy effect)
- Replace pseudo-code examples with JSON/MCP parameter schemas
- Consolidate tool table, add OCR column, add routing prompt guide
- Move OpenSSF Scorecard badge next to Codecov badge
- Fix ghost_mcp_guide routing prompt content
- Add BENCHMARKING.md and update CLAUDE.md with bench-report commands


### Features
- **cv**: Implement pure Go visual icon detection pipeline
- **ocr**: Add perceptual image hashing for robust scroll termination
- **ocr**: Implement asynchronous OCR pipeline for learn_screen
- Standardise logging and remove legacy GHOST_MCP_DEBUG flag
- Add element_types filter to get_learned_view
- Harden Visual ID workflow and precision routing
- Universal Visual ID ecosystem and precision routing
- **routing**: Prioritize Learn & Annotate flow and enhance get_annotated_view
- **find_elements**: Add widget_x/widget_y/checked for checkbox and radio elements
- **learner**: Expand checkbox/radio symbol detection and add IsCheckedSymbol
- Detect placeholder text in input fields via darkTextMaxLum + inputPlaceholders
- Wire DarkText pass into parallelFindText, learn_screen, and mergeOCRPasses
- **ocr**: Add DarkText preprocessing pass for dark text on coloured backgrounds
- Split fixture into normal and challenge pages
- Add ColorInverted OCR pass for white text on light coloured backgrounds
- Add scan_pages to find_elements and improve failure diagnostics
- Add actionable_elements to find_elements response
- Enable learning mode by default (opt-out with GHOST_MCP_LEARNING=0)
- Add server instructions directing Claude to read ghost_mcp_guide on connect
- Add ghost_mcp_guide routing prompt for AI tool selection
- Add -save flag to auto-commit benchmark result JSONs
- Add bench-report CLI tool with JSON storage and HTML report generation
- Add missing benchmark coverage for validate, audit, and handler packages


### Performance
- Eliminate per-call slice allocations in InferElementType helpers


## [deps-windows-v2] — 2026-04-03

### Bug Fixes
- HashImageFast now distinguishes colors with same brightness
- Correct test image color format in hashImageFast tests
- Use Go 1.25.8 and fix CodeQL security warning
- Downgrade Go version to 1.25 for CI compatibility
- Auto-try text variations for OCR failures (punctuation, multi-word)
- Correct TestTextSimilarity_ShortStrings assertion


### Dependency Updates
- **deps**: Bump step-security/harden-runner from 2.16.0 to 2.16.1
- **deps**: Bump actions/setup-go from 6.3.0 to 6.4.0


### Documentation
- Fix comment - learnScreen uses 4 OCR passes not 3
- Add OCR accuracy fixes documentation
- Strengthen tool descriptions to steer AI toward learning mode workflow
- Update CLAUDE.md for learning mode feature


### Features
- Add character whitelist, language support, and PSM modes to OCR
- Add slider element type for range controls
- Add checkbox, radio, dropdown, toggle element types
- Add 'input' element type for text field detection
- Add element type classification to find_elements output
- Add learning mode automation tools with stale view protection
- Integrate learning mode into OCR tool handlers
- Add learning mode MCP tools and screen discovery algorithm
- Add internal/learner package with view storage and element lookup
- Composite tools for faster UI automation


### Performance
- Add sync.Pool for zero-latency Tesseract caching
- Instantiate Tesseract dynamically for true OCR concurrency
- Switch OCR injection to Nearest-Neighbor BMP
- Bypass disk I/O in OCR automation pipelines


## [deps-windows-v1] — 2026-03-31

### Bug Fixes
- Return actual cursor position from click tools, not requested coords
- Add post-click delay so screenshots capture updated UI state
- Upgrade mcp-go and robotgo, fix CI pipeline


### Dependency Updates
- **deps**: Bump actions/setup-go from 5.6.0 to 6.3.0
- **deps**: Bump actions/checkout from 4.3.1 to 6.0.2
- **deps**: Bump codecov/codecov-action from 4.6.0 to 6.0.0
- **deps**: Bump github/codeql-action from 3.35.1 to 4.35.1
- **deps**: Bump github.com/go-vgo/robotgo from 0.110.4 to 1.0.1
- **deps**: Bump actions/upload-artifact from 4.6.2 to 7.0.0


### Documentation
- Convert ASCII diagrams to PlantUML with rendered images


### Features
- Scroll returns visible_text; x,y optional; add ReadImage API
- **ocr**: Inverted-image fallback for white-on-dark button text
- **ocr**: Add grayscale parameter to OCR tools
- **ocr**: Add grayscale + contrast stretch preprocessing
- **ocr**: Use PSM_SPARSE_TEXT for UI screenshot recognition
- Surface DPI scale factor in get_screen_size response


### Performance
- **screenshot**: Eliminate disk I/O, add JPEG quality option
- **ocr**: Reuse Tesseract client across calls (singleton)
- **ocr**: Use SetImageFromBytes to eliminate temp file round trip
- **ocr**: Single-pass grayscale with direct Pix slice access
- **ocr**: Drop redundant client.Text() recognition pass
- **ocr**: Increase scale factor to 3x for better UI text recognition
- Add region params to find_and_click and reduce OCR scale factor



