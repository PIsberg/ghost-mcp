// Package main implements the bench-report tool.
package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// BenchResult holds parsed data from one `go test -bench` output line.
type BenchResult struct {
	Name        string  `json:"name"`
	Iterations  int64   `json:"iterations"`
	NsPerOp     float64 `json:"ns_per_op"`
	BytesPerOp  int64   `json:"bytes_per_op,omitempty"`
	AllocsPerOp int64   `json:"allocs_per_op,omitempty"`
	MBPerSec    float64 `json:"mb_per_sec,omitempty"`
}

// RunRecord captures a full benchmark run for one package.
type RunRecord struct {
	Timestamp string        `json:"timestamp"`
	GitCommit string        `json:"git_commit"`
	GitBranch string        `json:"git_branch"`
	GoVersion string        `json:"go_version"`
	Package   string        `json:"package"`
	GOOS      string        `json:"goos"`
	GOARCH    string        `json:"goarch"`
	CPU       string        `json:"cpu"`
	Results   []BenchResult `json:"results"`
}

// parseBenchmarkOutput parses the stdout of `go test -bench -benchmem` and
// returns one BenchResult per benchmark line found. Lines that are not
// benchmark results (goos:, pkg:, PASS, etc.) are ignored.
//
// Example input line:
//
//	BenchmarkFindElement-16    5000000    240.3 ns/op    0 B/op    0 allocs/op
func parseBenchmarkOutput(r io.Reader) ([]BenchResult, map[string]string, error) {
	meta := map[string]string{}
	var results []BenchResult

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Capture metadata lines: "goos: windows", "pkg: ...", "cpu: ..."
		if key, val, ok := parseMetaLine(line); ok {
			meta[key] = val
			continue
		}

		// Benchmark result lines start with "Benchmark"
		if !strings.HasPrefix(line, "Benchmark") {
			continue
		}

		result, err := parseBenchLine(line)
		if err != nil {
			// Skip malformed lines (e.g. SKIP output mixed in)
			continue
		}
		results = append(results, result)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("scan error: %w", err)
	}
	return results, meta, nil
}

// parseMetaLine parses lines like "goos: windows" or "pkg: github.com/foo".
func parseMetaLine(line string) (key, val string, ok bool) {
	for _, prefix := range []string{"goos", "goarch", "pkg", "cpu"} {
		if strings.HasPrefix(line, prefix+": ") {
			return prefix, strings.TrimPrefix(line, prefix+": "), true
		}
	}
	return "", "", false
}

// parseBenchLine parses a single benchmark result line.
// Format: BenchmarkName[-N]  <iterations>  <ns/op> ns/op  [<B/op> B/op  <allocs/op> allocs/op]
func parseBenchLine(line string) (BenchResult, error) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return BenchResult{}, fmt.Errorf("too few fields: %q", line)
	}

	// Strip the -N suffix (goroutine count) from the benchmark name.
	name := fields[0]
	if idx := strings.LastIndex(name, "-"); idx > 0 {
		if _, err := strconv.Atoi(name[idx+1:]); err == nil {
			name = name[:idx]
		}
	}

	iterations, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return BenchResult{}, fmt.Errorf("parse iterations %q: %w", fields[1], err)
	}

	result := BenchResult{
		Name:       name,
		Iterations: iterations,
	}

	// Parse metric pairs: value unit, value unit, ...
	for i := 2; i+1 < len(fields); i += 2 {
		val, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			continue
		}
		unit := fields[i+1]
		switch unit {
		case "ns/op":
			result.NsPerOp = val
		case "B/op":
			result.BytesPerOp = int64(val)
		case "allocs/op":
			result.AllocsPerOp = int64(val)
		case "MB/s":
			result.MBPerSec = val
		}
	}

	if result.NsPerOp == 0 && result.MBPerSec == 0 {
		return BenchResult{}, fmt.Errorf("no timing found in line %q", line)
	}
	return result, nil
}
