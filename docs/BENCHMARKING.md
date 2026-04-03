# Benchmarking Guide

Ghost MCP ships a comprehensive benchmark suite and a reporting tool that generates interactive HTML reports for performance tracking across features, dependency bumps, and code changes.

## Running Benchmarks Directly

```bash
# All benchmark packages (unit-testable — no display required)
go test -bench=. -benchmem ./internal/validate/...
go test -bench=. -benchmem ./internal/audit/...
go test -bench=. -benchmem ./internal/learner/...
go test -bench=. -benchmem ./internal/ocr/...

# cmd/ghost-mcp benchmarks require robotgo CGo setup (see CLAUDE.md)
go test -bench=. -benchmem -short ./cmd/ghost-mcp/...

# Filter by name
go test -bench=BenchmarkOCR -benchmem -benchtime=5s ./...

# All packages at once
go test -bench=. -benchmem ./...
```

## Benchmark Coverage

| Package | Benchmarks | What's measured |
|---------|-----------|-----------------|
| `internal/validate` | 7 | `Coords`, `ScreenRegion`, `Text`, `Key` — valid and invalid paths |
| `internal/audit` | 6 | Hash-chain log writes, `computeEntryHash`, `sanitizeParams`, `VerifyLogFile` |
| `internal/learner` | 7 | `FindElement`, `FindAllElements`, `ScoreMatch`, `DeduplicateElements`, `InferElementType`, `AssociateLabels` |
| `internal/ocr` | 6 | `ReadImage` (grayscale + color), `PrepareParallelImageSet`, `ToGrayscaleContrast` |
| `cmd/ghost-mcp` | 13 | `parallelFindText`, `hashImageFast`, `textSimilarity`, `mergeOCRPasses`, `InferElementType`, `AssociateLabels` |

Total: **39 benchmarks** across 5 packages.

## Generating an HTML Report

The `bench-report` tool runs all benchmarks, saves JSON results, and produces an interactive HTML report.

```bash
# Basic run — runs all benchmark packages, opens report.html in browser
go run ./cmd/bench-report/

# Custom benchtime and count for more stable numbers
go run ./cmd/bench-report/ -benchtime=2s -count=5

# Filter to specific benchmarks
go run ./cmd/bench-report/ -bench=BenchmarkOCR

# Specific packages only
go run ./cmd/bench-report/ -packages="./internal/validate/...,./internal/learner/..."

# Show only last 3 runs per package in the report
go run ./cmd/bench-report/ -compare=3

# Regenerate HTML from stored results without re-running benchmarks
go run ./cmd/bench-report/ -no-run

# Save report to a custom path
go run ./cmd/bench-report/ -out=docs/perf-report.html
```

The report is written to `benchmarks/report.html` (self-contained, no internet required after first load if Chart.js is cached).

### Report Sections

- **Runs**: metadata for each stored run (branch, commit, timestamp, Go version, CPU)
- **Per-package bar charts**: ns/op and allocs/op for the latest run of each benchmark
- **Summary table**: ns/op, B/op, allocs/op per benchmark with a delta badge showing % change vs the previous run (green = faster, red = slower)
- **Trend modals**: click the `↗ trend` link on any benchmark row to see a line chart of ns/op across all stored runs

## Stored Results

Each run is persisted as a JSON file in `benchmarks/results/`:

```
benchmarks/results/
  2026-04-03_10-00-00_abc1234_github_com_ghost_mcp_internal_validate.json
  2026-04-03_10-00-00_abc1234_github_com_ghost_mcp_internal_learner.json
  ...
```

Files include: `timestamp`, `git_commit`, `git_branch`, `go_version`, `package`, `goos`, `goarch`, `cpu`, and the full list of `BenchResult` entries.

These files are committed to git so the history accumulates over time, making it easy to compare performance before and after a feature branch or dependency update.

## Comparing Dependency Updates

To measure whether bumping a dependency improved performance:

```bash
# 1. On main / before the bump — run the benchmark suite
go run ./cmd/bench-report/ -benchtime=2s -count=5

# 2. Update go.mod / go.sum (e.g. bump gosseract)
go get github.com/otiai10/gosseract/v2@latest
go mod tidy

# 3. Run again — the report will show deltas
go run ./cmd/bench-report/ -benchtime=2s -count=5
```

The HTML report will automatically pick up both runs and show the delta column.

## Windows CGo Setup

The `cmd/ghost-mcp` package benchmarks require MinGW and Tesseract via vcpkg:

```bat
# Use test_runner.bat which sets up the environment automatically
test_runner.bat

# Or set manually before running go test
set CGO_CPPFLAGS=-I%USERPROFILE%\vcpkg\installed\x64-mingw-dynamic\include
set CGO_LDFLAGS=-L%USERPROFILE%\vcpkg\installed\x64-mingw-dynamic\lib
set PATH=C:\ProgramData\mingw64\mingw64\bin;%USERPROFILE%\vcpkg\installed\x64-mingw-dynamic\bin;%PATH%
go test -bench=. -benchmem -short ./cmd/ghost-mcp/...
```

The `bench-report` tool will attempt to run `cmd/ghost-mcp` benchmarks; if CGo is not available it prints a warning and skips that package.
