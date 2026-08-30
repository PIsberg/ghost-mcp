# Ghost MCP Documentation Index

Start with the [project README](../README.md) for installation, configuration, and the
tool catalogue. This index maps every document in `docs/` so you can find the right
sub-document without opening each one.

## Using Ghost MCP

| Document | What it covers |
|----------|----------------|
| [USAGE.md](USAGE.md) | Step-by-step usage guide with examples against the interactive test fixture |
| [routing_prompt.md](routing_prompt.md) | Tool routing guide for AI clients: safety rules, decision tables, recommended workflows. Mirrors the MCP prompt in `cmd/ghost-mcp/prompts.go`; keep the two in sync |
| [WORKFLOW_TOOL.md](WORKFLOW_TOOL.md) | The `execute_workflow` tool: running multi-step sequences against a single learned view |

## Architecture and design

| Document | What it covers |
|----------|----------------|
| [ARCHITECTURE.md](ARCHITECTURE.md) | Internal architecture, request flow, and design decisions |
| [diagrams/](diagrams/) | PlantUML sources and rendered PNGs: system architecture, request flow, tool handling, concurrency, class diagrams, startup sequence |

## OCR and learning mode

| Document | What it covers |
|----------|----------------|
| [OCR_FEATURES.md](OCR_FEATURES.md) | OCR feature guide: character whitelist, preprocessing passes, matching options |
| [TESSERACT_FEATURES.md](TESSERACT_FEATURES.md) | Which Tesseract capabilities are used, and candidates for future use |
| [LEARNING_MODE_IMPROVEMENTS.md](LEARNING_MODE_IMPROVEMENTS.md) | Why learning mode adoption was inconsistent and the changes that fixed it |
| [STALE_VIEW_FIX.md](STALE_VIEW_FIX.md) | How stale learned views after page refresh are detected and rebuilt |
| [OCR_ACCURACY_FIXES.md](OCR_ACCURACY_FIXES.md) | Root-cause analysis and fixes for OCR misses found in real usage logs |

## Testing and quality

| Document | What it covers |
|----------|----------------|
| [TESTING.md](TESTING.md) | Main testing guide: unit tests, integration tests, the test fixture, and AI judge testing |
| [LEARNING_MODE_TESTING.md](LEARNING_MODE_TESTING.md) | Running and interpreting the learning mode accuracy tests |
| [AI_JUDGE_EVALUATION.md](AI_JUDGE_EVALUATION.md) | Performance evaluation of GUI element identification using the Gemini-based AI judge |
| [TEST_RESULTS.md](TEST_RESULTS.md) | Recorded learning mode test results snapshot |
| [BENCHMARKING.md](BENCHMARKING.md) | Benchmark suite and the HTML report generator (`cmd/bench-report`) |

## Contributing and maintenance

| Document | What it covers |
|----------|----------------|
| [FORMATTING.md](FORMATTING.md) | Code formatting rules and the pre-commit hook setup |
| [REBUILD_WINDOWS_DEPS.md](REBUILD_WINDOWS_DEPS.md) | Rebuilding the pre-built Windows dependency bundle used by CI |
