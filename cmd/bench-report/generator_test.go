package main

import (
	"strings"
	"testing"
	"time"
)

func sampleRuns() []RunRecord {
	return []RunRecord{
		{
			Timestamp: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC).Format(time.RFC3339),
			GitCommit: "abc1234def567",
			GitBranch: "main",
			GoVersion: "go version go1.22.0 windows/amd64",
			Package:   "github.com/ghost-mcp/internal/validate",
			GOOS:      "windows",
			GOARCH:    "amd64",
			CPU:       "Intel i7",
			Results: []BenchResult{
				{Name: "BenchmarkCoords_Valid", Iterations: 1000000, NsPerOp: 1.5, BytesPerOp: 0, AllocsPerOp: 0},
				{Name: "BenchmarkText_Short", Iterations: 500000, NsPerOp: 2.8, BytesPerOp: 0, AllocsPerOp: 0},
				{Name: "BenchmarkKey_Valid", Iterations: 200000, NsPerOp: 6.0, BytesPerOp: 0, AllocsPerOp: 0},
			},
		},
		{
			Timestamp: time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC).Format(time.RFC3339),
			GitCommit: "def5678abc123",
			GitBranch: "feature/benchmark-reports",
			GoVersion: "go version go1.22.0 windows/amd64",
			Package:   "github.com/ghost-mcp/internal/validate",
			GOOS:      "windows",
			GOARCH:    "amd64",
			CPU:       "Intel i7",
			Results: []BenchResult{
				{Name: "BenchmarkCoords_Valid", Iterations: 1200000, NsPerOp: 1.2, BytesPerOp: 0, AllocsPerOp: 0},
				{Name: "BenchmarkText_Short", Iterations: 600000, NsPerOp: 3.1, BytesPerOp: 0, AllocsPerOp: 0},
				{Name: "BenchmarkKey_Valid", Iterations: 250000, NsPerOp: 5.5, BytesPerOp: 0, AllocsPerOp: 0},
			},
		},
	}
}

func TestGenerateHTMLReport_ContainsExpectedContent(t *testing.T) {
	runs := sampleRuns()
	var sb strings.Builder
	if err := generateHTMLReport(&sb, runs); err != nil {
		t.Fatalf("generateHTMLReport: %v", err)
	}
	html := sb.String()

	checks := []string{
		"<!DOCTYPE html>",
		"Ghost MCP",
		"Benchmark Report",
		"chart.js",
		"internal/validate",
		"Coords_Valid",
		"Text_Short",
		"Key_Valid",
	}
	for _, want := range checks {
		if !strings.Contains(html, want) {
			t.Errorf("expected HTML to contain %q", want)
		}
	}
}

func TestGenerateHTMLReport_EmptyRuns(t *testing.T) {
	var sb strings.Builder
	if err := generateHTMLReport(&sb, nil); err != nil {
		t.Fatalf("generateHTMLReport with nil runs: %v", err)
	}
	html := sb.String()
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("expected valid HTML even with no runs")
	}
	if !strings.Contains(html, "No benchmark data") {
		t.Error("expected empty-state message")
	}
}

func TestGenerateHTMLReport_DeltaCalculation(t *testing.T) {
	runs := sampleRuns()
	var sb strings.Builder
	if err := generateHTMLReport(&sb, runs); err != nil {
		t.Fatalf("generateHTMLReport: %v", err)
	}
	html := sb.String()
	// BenchmarkCoords_Valid went from 1.5 → 1.2 ns/op: 20% faster
	if !strings.Contains(html, "faster") {
		t.Error("expected at least one 'faster' delta label")
	}
	// BenchmarkText_Short went from 2.8 → 3.1 ns/op: slower
	if !strings.Contains(html, "slower") {
		t.Error("expected at least one 'slower' delta label")
	}
}

func TestGenerateHTMLReport_TrendModal(t *testing.T) {
	runs := sampleRuns()
	var sb strings.Builder
	if err := generateHTMLReport(&sb, runs); err != nil {
		t.Fatalf("generateHTMLReport: %v", err)
	}
	html := sb.String()
	// With 2 runs there should be trend modals
	if !strings.Contains(html, "trend-modal") {
		t.Error("expected trend modal elements for multi-run data")
	}
	if !strings.Contains(html, "trend") {
		t.Error("expected trend button")
	}
}

func TestBuildReportData_PackageOrdering(t *testing.T) {
	runs := []RunRecord{
		{Package: "pkg/b", Results: []BenchResult{{Name: "BenchmarkB", NsPerOp: 1}}},
		{Package: "pkg/a", Results: []BenchResult{{Name: "BenchmarkA", NsPerOp: 1}}},
		{Package: "pkg/b", Results: []BenchResult{{Name: "BenchmarkB", NsPerOp: 1}}},
	}
	data := buildReportData(runs)
	if len(data.Packages) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(data.Packages))
	}
	if data.Packages[0].Package != "pkg/b" {
		t.Errorf("expected first package to be pkg/b (insertion order), got %q", data.Packages[0].Package)
	}
}

func TestBuildReportData_DeltaDirection(t *testing.T) {
	runs := []RunRecord{
		{Package: "pkg/x", Results: []BenchResult{{Name: "BenchmarkFoo", NsPerOp: 100}}},
		{Package: "pkg/x", Results: []BenchResult{{Name: "BenchmarkFoo", NsPerOp: 80}}},
	}
	data := buildReportData(runs)
	if len(data.Packages) != 1 {
		t.Fatalf("expected 1 package")
	}
	bh := data.Packages[0].Benchmarks[0]
	if !bh.HasDelta {
		t.Error("expected HasDelta=true")
	}
	if !bh.Faster {
		t.Error("expected Faster=true (80 < 100)")
	}
	// Delta = (80-100)/100*100 = -20%
	if bh.Delta >= 0 {
		t.Errorf("expected negative delta (improvement), got %.2f", bh.Delta)
	}
}

func TestLatestPoint_FindsLastPresent(t *testing.T) {
	points := []HistoryPoint{
		{Present: true, NsPerOp: 10},
		{Present: false},
		{Present: true, NsPerOp: 20},
		{Present: false},
	}
	pt := latestPoint(points)
	if !pt.Present {
		t.Error("expected a present point")
	}
	if pt.NsPerOp != 20 {
		t.Errorf("expected 20 ns/op, got %f", pt.NsPerOp)
	}
}

func TestLatestPoint_AllAbsent(t *testing.T) {
	points := []HistoryPoint{{Present: false}, {Present: false}}
	pt := latestPoint(points)
	if pt.Present {
		t.Error("expected no present point")
	}
}

func TestFormatNs(t *testing.T) {
	cases := []struct {
		ns   float64
		want string
	}{
		{0.5, "0.5 ns"},
		{999, "999.0 ns"},
		{1000, "1.00 µs"},
		{1_500_000, "1.50 ms"},
		{2_000_000_000, "2.000 s"},
	}
	for _, tc := range cases {
		got := formatNs(tc.ns)
		if got != tc.want {
			t.Errorf("formatNs(%.0f): want %q, got %q", tc.ns, tc.want, got)
		}
	}
}

func TestShortPkg(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"github.com/ghost-mcp/internal/validate", "internal/validate"},
		{"github.com/ghost-mcp/cmd/bench-report", "cmd/bench-report"},
		{"simple", "simple"},
	}
	for _, tc := range cases {
		got := shortPkg(tc.input)
		if got != tc.want {
			t.Errorf("shortPkg(%q): want %q, got %q", tc.input, tc.want, got)
		}
	}
}

func TestSafeID(t *testing.T) {
	got := safeID("github.com/foo/bar-baz")
	for _, c := range got {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			t.Errorf("safeID produced non-safe character %q in %q", c, got)
		}
	}
}

func TestJsonNumbers(t *testing.T) {
	points := []HistoryPoint{
		{Present: true, NsPerOp: 1.5},
		{Present: false},
		{Present: true, NsPerOp: 3.0},
	}
	got := jsonNumbers(points)
	if got != "[1.50,null,3.00]" {
		t.Errorf("jsonNumbers: got %q", got)
	}
}

func TestBenchLabels(t *testing.T) {
	benchmarks := []BenchmarkHistory{
		{ShortName: "Foo"},
		{ShortName: "Bar"},
	}
	got := benchLabels(benchmarks)
	if got != `["Foo","Bar"]` {
		t.Errorf("benchLabels: got %q", got)
	}
}
