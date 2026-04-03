// bench-report runs the Ghost MCP benchmark suite, saves JSON results, and
// generates a self-contained HTML report with Chart.js visualisations.
//
// Usage:
//
//	go run ./cmd/bench-report/                       # full run, open report
//	go run ./cmd/bench-report/ -bench=BenchmarkOCR  # filter benchmarks
//	go run ./cmd/bench-report/ -benchtime=2s -count=5
//	go run ./cmd/bench-report/ -no-run               # regenerate HTML only
//
// Flags:
//
//	-bench string      Benchmark filter regexp (default ".")
//	-benchtime dur     Time per benchmark (default "1s")
//	-count int         Number of benchmark runs per test (default 3)
//	-out string        Output HTML path (default "benchmarks/report.html")
//	-results-dir str   Directory for JSON results (default "benchmarks/results")
//	-packages string   Comma-separated package patterns (default: all benchmarked packages)
//	-compare int       Maximum number of past runs to include in report (0 = all)
//	-no-run            Skip running benchmarks; regenerate HTML from stored results only
//	-open              Open the report in a browser after generation (default true)
//	-save              Git-commit the new result JSON files so history is preserved
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// defaultPackages are the benchmark packages that don't require a display or CGo.
// cmd/ghost-mcp requires robotgo/CGo and is excluded from the default set so
// that the tool works without a special build environment.
var defaultPackages = []string{
	"./internal/validate/...",
	"./internal/audit/...",
	"./internal/learner/...",
	"./internal/ocr/...",
	"./cmd/ghost-mcp/...",
}

func main() {
	bench := flag.String("bench", ".", "Benchmark filter regexp")
	benchtime := flag.String("benchtime", "1s", "Time per benchmark (e.g. 1s, 500ms)")
	count := flag.Int("count", 3, "Number of benchmark runs per test binary")
	out := flag.String("out", filepath.Join("benchmarks", "report.html"), "Output HTML file path")
	resultsDir := flag.String("results-dir", filepath.Join("benchmarks", "results"), "Directory for JSON result files")
	packages := flag.String("packages", "", "Comma-separated package patterns (default: all benchmark packages)")
	compare := flag.Int("compare", 0, "Max past runs to include (0 = all)")
	noRun := flag.Bool("no-run", false, "Skip running benchmarks; regenerate HTML from stored results only")
	openReport := flag.Bool("open", true, "Open the report in a browser after generation")
	save := flag.Bool("save", false, "Git-commit new result JSON files after the run")
	flag.Parse()

	if err := run(*bench, *benchtime, *count, *out, *resultsDir, *packages, *compare, *noRun, *openReport, *save); err != nil {
		fmt.Fprintf(os.Stderr, "bench-report: %v\n", err)
		os.Exit(1)
	}
}

func run(bench, benchtime string, count int, out, resultsDir, packages string, compare int, noRun, openReport, save bool) error {
	// Ensure output directories exist.
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return fmt.Errorf("create results dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	pkgList := defaultPackages
	if packages != "" {
		pkgList = strings.Split(packages, ",")
		for i, p := range pkgList {
			pkgList[i] = strings.TrimSpace(p)
		}
	}

	var newRuns []RunRecord
	var newFiles []string // paths of JSON files written this run

	if !noRun {
		goVersion := goVersion()
		gitCommit := gitInfo("commit")
		gitBranch := gitInfo("branch")
		timestamp := time.Now().UTC().Format(time.RFC3339)

		fmt.Printf("bench-report: running benchmarks on %d package(s)\n", len(pkgList))
		fmt.Printf("  bench=%s  benchtime=%s  count=%d\n\n", bench, benchtime, count)

		for _, pkg := range pkgList {
			fmt.Printf("  → %s\n", pkg)
			output, err := runBenchmarks(pkg, bench, benchtime, count)
			if err != nil {
				fmt.Fprintf(os.Stderr, "    WARN: %v (skipping)\n", err)
				continue
			}

			results, meta, err := parseBenchmarkOutput(bytes.NewReader(output))
			if err != nil {
				fmt.Fprintf(os.Stderr, "    WARN: parse error: %v\n", err)
				continue
			}
			if len(results) == 0 {
				fmt.Printf("    (no benchmarks found)\n")
				continue
			}
			fmt.Printf("    %d benchmark result(s)\n", len(results))

			resolvedPkg := meta["pkg"]
			if resolvedPkg == "" {
				resolvedPkg = pkg
			}

			rec := RunRecord{
				Timestamp: timestamp,
				GitCommit: gitCommit,
				GitBranch: gitBranch,
				GoVersion: goVersion,
				Package:   resolvedPkg,
				GOOS:      meta["goos"],
				GOARCH:    meta["goarch"],
				CPU:       meta["cpu"],
				Results:   results,
			}
			newRuns = append(newRuns, rec)

			// Save JSON result file.
			shortSHA := gitCommit
			if len(shortSHA) > 7 {
				shortSHA = shortSHA[:7]
			}
			safeTS := strings.NewReplacer(":", "-", "T", "_", "Z", "").Replace(timestamp)
			fname := fmt.Sprintf("%s_%s_%s.json", safeTS, shortSHA, safeID(resolvedPkg))
			fpath := filepath.Join(resultsDir, fname)
			if err := saveJSON(fpath, rec); err != nil {
				fmt.Fprintf(os.Stderr, "    WARN: save JSON: %v\n", err)
			} else {
				newFiles = append(newFiles, fpath)
			}
		}
		fmt.Println()

		if save && len(newFiles) > 0 {
			if err := gitCommitResults(newFiles, gitCommit, gitBranch); err != nil {
				fmt.Fprintf(os.Stderr, "bench-report: git commit failed: %v\n", err)
			}
		}
	}

	// Load all stored results.
	allRuns, err := loadResults(resultsDir)
	if err != nil {
		return fmt.Errorf("load results: %w", err)
	}

	if compare > 0 {
		allRuns = limitRuns(allRuns, compare)
	}

	if len(allRuns) == 0 {
		fmt.Println("bench-report: no benchmark results to report.")
		return nil
	}

	// Generate HTML report.
	f, err := os.Create(out)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer f.Close()

	if err := generateHTMLReport(f, allRuns); err != nil {
		return fmt.Errorf("generate HTML: %w", err)
	}

	absOut, _ := filepath.Abs(out)
	fmt.Printf("bench-report: report written to %s\n", absOut)

	if openReport {
		openBrowser("file://" + filepath.ToSlash(absOut))
	}
	return nil
}

// runBenchmarks runs `go test -bench` on one package and returns raw output.
func runBenchmarks(pkg, bench, benchtime string, count int) ([]byte, error) {
	args := []string{
		"test",
		"-bench=" + bench,
		"-benchmem",
		"-benchtime=" + benchtime,
		fmt.Sprintf("-count=%d", count),
		"-run=^$", // skip all non-benchmark tests
		"-short",
		pkg,
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = repoRoot()

	// Pass the current environment so CGO_CPPFLAGS etc. are inherited.
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()
	if err != nil {
		// A non-zero exit can mean tests were skipped or the package failed to
		// build.  Return the output so the caller can decide.
		if len(out) == 0 {
			return nil, fmt.Errorf("go test failed: %w", err)
		}
		// If the output contains benchmark lines, treat it as a partial success.
		if strings.Contains(string(out), "Benchmark") {
			return out, nil
		}
		return nil, fmt.Errorf("go test: %w\n%s", err, out)
	}
	return out, nil
}

// saveJSON writes a RunRecord to a file as JSON.
func saveJSON(path string, rec RunRecord) error {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// loadResults reads all JSON files from dir and returns them sorted by Timestamp.
func loadResults(dir string) ([]RunRecord, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var runs []RunRecord
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || e.Name() == ".gitkeep" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: read %s: %v\n", e.Name(), err)
			continue
		}
		var rec RunRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			fmt.Fprintf(os.Stderr, "warn: parse %s: %v\n", e.Name(), err)
			continue
		}
		runs = append(runs, rec)
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].Timestamp < runs[j].Timestamp
	})
	return runs, nil
}

// limitRuns keeps only the last n runs per package.
func limitRuns(runs []RunRecord, n int) []RunRecord {
	// Group by package, then keep last n.
	byPkg := map[string][]RunRecord{}
	for _, r := range runs {
		byPkg[r.Package] = append(byPkg[r.Package], r)
	}
	var out []RunRecord
	for _, pkgRuns := range byPkg {
		if len(pkgRuns) > n {
			pkgRuns = pkgRuns[len(pkgRuns)-n:]
		}
		out = append(out, pkgRuns...)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp < out[j].Timestamp
	})
	return out
}

// gitCommitResults stages the given result files and creates a git commit.
func gitCommitResults(files []string, commit, branch string) error {
	root := repoRoot()

	// Stage only the new result files.
	addArgs := append([]string{"add", "--"}, files...)
	if out, err := exec.Command("git", addArgs...).Output(); err != nil {
		return fmt.Errorf("git add: %w\n%s", err, out)
	}

	shortSHA := commit
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}
	msg := fmt.Sprintf("chore: save benchmark results for %s @ %s\n\nGenerated by bench-report.", branch, shortSHA)
	cmd := exec.Command("git", "commit", "-m", msg)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %w\n%s", err, out)
	}

	fmt.Printf("bench-report: committed %d result file(s) to git\n", len(files))
	fmt.Println("  Push with: git push")
	return nil
}

// goVersion returns the Go toolchain version string.
func goVersion() string {
	out, err := exec.Command("go", "version").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// gitInfo returns the current git commit hash ("commit") or branch name ("branch").
func gitInfo(what string) string {
	var args []string
	switch what {
	case "commit":
		args = []string{"rev-parse", "HEAD"}
	case "branch":
		args = []string{"rev-parse", "--abbrev-ref", "HEAD"}
	default:
		return ""
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot()
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// repoRoot returns the repository root (two directories up from this file).
func repoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// openBrowser attempts to open url in the default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
