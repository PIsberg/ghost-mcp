# Changelog

All notable changes to ghost-mcp are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/);
versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.5.0] — 2026-04-10

> **Test release.** This is the first versioned release of ghost-mcp, published
> to validate the GoReleaser build pipeline (binaries, Windows zip with bundled
> DLLs, Linux deb/rpm packages, Scoop manifest). The feature set is functional
> but the API surface may still change before 1.0.

### Features

- **cv**: implement pure Go visual icon detection pipeline
- **ocr**: add perceptual image hashing for robust scroll termination
- **ocr**: implement asynchronous OCR pipeline for learn_screen
- Standardise logging and remove legacy `GHOST_MCP_DEBUG` flag
- Add `element_types` filter to `get_learned_view`

### Bug Fixes

- **test**: relax overly aggressive dHash test threshold
- Make short-mode test suite pass cleanly
- Update `handleMoveMouse` to return JSON instead of plain text
- Resolve prompt and OCR test regressions by restoring legacy keywords
- Resolve double-typing bug and enable screenshot persistence
