package main

import (
	"strings"
	"testing"
)

func TestParseBenchmarkOutput_FullBlock(t *testing.T) {
	input := `goos: windows
goarch: amd64
pkg: github.com/ghost-mcp/internal/validate
cpu: 12th Gen Intel(R) Core(TM) i7-1260P
BenchmarkCoords_Valid-16          	232785962	         1.021 ns/op	       0 B/op	       0 allocs/op
BenchmarkCoords_Invalid-16        	  2244345	       107.0 ns/op	      56 B/op	       3 allocs/op
BenchmarkText_Long-16             	   183163	      1300 ns/op	       0 B/op	       0 allocs/op
PASS
ok  	github.com/ghost-mcp/internal/validate	3.187s
`
	results, meta, err := parseBenchmarkOutput(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if meta["goos"] != "windows" {
		t.Errorf("goos: want windows, got %q", meta["goos"])
	}
	if meta["pkg"] != "github.com/ghost-mcp/internal/validate" {
		t.Errorf("pkg: unexpected %q", meta["pkg"])
	}

	r := results[0]
	if r.Name != "BenchmarkCoords_Valid" {
		t.Errorf("name: want BenchmarkCoords_Valid, got %q", r.Name)
	}
	if r.Iterations != 232785962 {
		t.Errorf("iterations: want 232785962, got %d", r.Iterations)
	}
	if r.NsPerOp != 1.021 {
		t.Errorf("ns/op: want 1.021, got %f", r.NsPerOp)
	}
	if r.BytesPerOp != 0 {
		t.Errorf("B/op: want 0, got %d", r.BytesPerOp)
	}
	if r.AllocsPerOp != 0 {
		t.Errorf("allocs/op: want 0, got %d", r.AllocsPerOp)
	}

	r2 := results[1]
	if r2.Name != "BenchmarkCoords_Invalid" {
		t.Errorf("name: want BenchmarkCoords_Invalid, got %q", r2.Name)
	}
	if r2.NsPerOp != 107.0 {
		t.Errorf("ns/op: want 107.0, got %f", r2.NsPerOp)
	}
	if r2.BytesPerOp != 56 {
		t.Errorf("B/op: want 56, got %d", r2.BytesPerOp)
	}
	if r2.AllocsPerOp != 3 {
		t.Errorf("allocs/op: want 3, got %d", r2.AllocsPerOp)
	}
}

func TestParseBenchmarkOutput_StripsGoroutineCount(t *testing.T) {
	input := `BenchmarkFoo-8    1000    500 ns/op    0 B/op    0 allocs/op`
	results, _, err := parseBenchmarkOutput(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "BenchmarkFoo" {
		t.Errorf("name: want BenchmarkFoo, got %q", results[0].Name)
	}
}

func TestParseBenchmarkOutput_MBPerSec(t *testing.T) {
	input := `BenchmarkReadBytes-16    50000    25000 ns/op    128 B/op    2 allocs/op    40.00 MB/s`
	results, _, err := parseBenchmarkOutput(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].MBPerSec != 40.0 {
		t.Errorf("MB/s: want 40.0, got %f", results[0].MBPerSec)
	}
}

func TestParseBenchmarkOutput_EmptyInput(t *testing.T) {
	results, _, err := parseBenchmarkOutput(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestParseBenchmarkOutput_SkipsNonBenchmarkLines(t *testing.T) {
	input := `
--- FAIL: TestFoo (0.01s)
PASS
ok github.com/foo 1.0s
BenchmarkBar-4    100    9999 ns/op    0 B/op    0 allocs/op
`
	results, _, err := parseBenchmarkOutput(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "BenchmarkBar" {
		t.Errorf("expected BenchmarkBar, got %q", results[0].Name)
	}
}

func TestParseBenchLine_Valid(t *testing.T) {
	cases := []struct {
		line     string
		wantName string
		wantNs   float64
		wantB    int64
		wantA    int64
	}{
		{
			"BenchmarkFoo-16    1000000    1.5 ns/op    0 B/op    0 allocs/op",
			"BenchmarkFoo", 1.5, 0, 0,
		},
		{
			"BenchmarkBar-4    5000    1234567.89 ns/op    1024 B/op    12 allocs/op",
			"BenchmarkBar", 1234567.89, 1024, 12,
		},
	}
	for _, tc := range cases {
		r, err := parseBenchLine(tc.line)
		if err != nil {
			t.Errorf("%q: unexpected error %v", tc.line, err)
			continue
		}
		if r.Name != tc.wantName {
			t.Errorf("name: want %q, got %q", tc.wantName, r.Name)
		}
		if r.NsPerOp != tc.wantNs {
			t.Errorf("ns/op: want %f, got %f", tc.wantNs, r.NsPerOp)
		}
		if r.BytesPerOp != tc.wantB {
			t.Errorf("B/op: want %d, got %d", tc.wantB, r.BytesPerOp)
		}
		if r.AllocsPerOp != tc.wantA {
			t.Errorf("allocs/op: want %d, got %d", tc.wantA, r.AllocsPerOp)
		}
	}
}

func TestParseBenchLine_TooFewFields(t *testing.T) {
	_, err := parseBenchLine("BenchmarkFoo 100")
	if err == nil {
		t.Error("expected error for too-few-fields line")
	}
}

func TestParseMetaLine(t *testing.T) {
	cases := []struct {
		line    string
		wantKey string
		wantVal string
		wantOK  bool
	}{
		{"goos: linux", "goos", "linux", true},
		{"goarch: arm64", "goarch", "arm64", true},
		{"pkg: github.com/foo/bar", "pkg", "github.com/foo/bar", true},
		{"cpu: Intel something", "cpu", "Intel something", true},
		{"PASS", "", "", false},
		{"BenchmarkFoo-8    1000    1 ns/op", "", "", false},
	}
	for _, tc := range cases {
		key, val, ok := parseMetaLine(tc.line)
		if ok != tc.wantOK {
			t.Errorf("%q: ok want %v, got %v", tc.line, tc.wantOK, ok)
		}
		if ok && key != tc.wantKey {
			t.Errorf("%q: key want %q, got %q", tc.line, tc.wantKey, key)
		}
		if ok && val != tc.wantVal {
			t.Errorf("%q: val want %q, got %q", tc.line, tc.wantVal, val)
		}
	}
}
