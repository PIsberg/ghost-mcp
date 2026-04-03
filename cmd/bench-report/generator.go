package main

import (
	"fmt"
	"html"
	"io"
	"math"
	"sort"
	"strings"
	"text/template"
	"time"
)

// ReportData is passed to the HTML template.
type ReportData struct {
	GeneratedAt string
	Runs        []RunRecord
	Packages    []PackageData
}

// PackageData groups benchmark results for one package across all runs.
type PackageData struct {
	Package    string
	ShortPkg   string
	Benchmarks []BenchmarkHistory
}

// BenchmarkHistory holds all data points for one named benchmark.
type BenchmarkHistory struct {
	Name      string
	ShortName string
	Points    []HistoryPoint
	Delta     float64
	HasDelta  bool
	Faster    bool
}

// HistoryPoint is one run's measurement for a benchmark.
type HistoryPoint struct {
	RunLabel    string
	NsPerOp     float64
	BytesPerOp  int64
	AllocsPerOp int64
	Present     bool
}

// generateHTMLReport writes a self-contained HTML benchmark report to w.
func generateHTMLReport(w io.Writer, runs []RunRecord) error {
	data := buildReportData(runs)

	funcMap := template.FuncMap{
		"formatNs":          formatNs,
		"jsonNumbers":       jsonNumbers,
		"jsonStrings":       jsonStrings,
		"safeID":            safeID,
		"absf":              math.Abs,
		"printf":            fmt.Sprintf,
		"latestPoint":       latestPoint,
		"benchLabels":       benchLabels,
		"benchLatestNs":     benchLatestNs,
		"benchLatestAllocs": benchLatestAllocs,
		"shortCommit":       shortCommit,
	}

	t, err := template.New("report").Funcs(funcMap).Parse(reportTemplate)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}
	return t.Execute(w, data)
}

func buildReportData(runs []RunRecord) ReportData {
	pkgSeen := map[string]int{}
	var pkgOrder []string
	for _, run := range runs {
		if _, ok := pkgSeen[run.Package]; !ok {
			pkgSeen[run.Package] = len(pkgOrder)
			pkgOrder = append(pkgOrder, run.Package)
		}
	}

	pkgData := make([]PackageData, 0, len(pkgOrder))
	for _, pkg := range pkgOrder {
		benchSeen := map[string]int{}
		var benchOrder []string
		for _, run := range runs {
			if run.Package != pkg {
				continue
			}
			for _, r := range run.Results {
				if _, ok := benchSeen[r.Name]; !ok {
					benchSeen[r.Name] = len(benchOrder)
					benchOrder = append(benchOrder, r.Name)
				}
			}
		}

		histories := make([]BenchmarkHistory, 0, len(benchOrder))
		for _, name := range benchOrder {
			bh := BenchmarkHistory{
				Name:      name,
				ShortName: strings.TrimPrefix(name, "Benchmark"),
			}
			var prevNs float64
			hasPrev := false
			for _, run := range runs {
				if run.Package != pkg {
					continue
				}
				label := runLabel(run)
				pt := HistoryPoint{RunLabel: label}
				for _, r := range run.Results {
					if r.Name == name {
						pt.NsPerOp = r.NsPerOp
						pt.BytesPerOp = r.BytesPerOp
						pt.AllocsPerOp = r.AllocsPerOp
						pt.Present = true
						break
					}
				}
				bh.Points = append(bh.Points, pt)
				if pt.Present {
					if hasPrev && prevNs > 0 {
						bh.Delta = (pt.NsPerOp - prevNs) / prevNs * 100
						bh.HasDelta = true
						bh.Faster = pt.NsPerOp < prevNs
					}
					prevNs = pt.NsPerOp
					hasPrev = true
				}
			}
			histories = append(histories, bh)
		}

		sort.Slice(histories, func(i, j int) bool {
			return latestNs(histories[i]) > latestNs(histories[j])
		})

		pkgData = append(pkgData, PackageData{
			Package:    pkg,
			ShortPkg:   shortPkg(pkg),
			Benchmarks: histories,
		})
	}

	return ReportData{
		GeneratedAt: time.Now().Format("2006-01-02 15:04:05 MST"),
		Runs:        runs,
		Packages:    pkgData,
	}
}

func latestNs(bh BenchmarkHistory) float64 {
	for i := len(bh.Points) - 1; i >= 0; i-- {
		if bh.Points[i].Present {
			return bh.Points[i].NsPerOp
		}
	}
	return 0
}

func latestPoint(points []HistoryPoint) HistoryPoint {
	for i := len(points) - 1; i >= 0; i-- {
		if points[i].Present {
			return points[i]
		}
	}
	return HistoryPoint{}
}

func runLabel(r RunRecord) string {
	commit := r.GitCommit
	if len(commit) > 7 {
		commit = commit[:7]
	}
	ts := r.Timestamp
	if len(ts) >= 10 {
		ts = ts[:10]
	}
	if r.GitBranch != "" {
		return html.EscapeString(fmt.Sprintf("%s @ %s %s", r.GitBranch, commit, ts))
	}
	return html.EscapeString(fmt.Sprintf("%s %s", commit, ts))
}

func shortPkg(pkg string) string {
	parts := strings.Split(pkg, "/")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return pkg
}

func shortCommit(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

func safeID(s string) string {
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func formatNs(ns float64) string {
	switch {
	case ns < 1000:
		return fmt.Sprintf("%.1f ns", ns)
	case ns < 1_000_000:
		return fmt.Sprintf("%.2f µs", ns/1000)
	case ns < 1_000_000_000:
		return fmt.Sprintf("%.2f ms", ns/1_000_000)
	default:
		return fmt.Sprintf("%.3f s", ns/1_000_000_000)
	}
}

func jsonNumbers(points []HistoryPoint) string {
	parts := make([]string, len(points))
	for i, p := range points {
		if p.Present {
			parts[i] = fmt.Sprintf("%.2f", p.NsPerOp)
		} else {
			parts[i] = "null"
		}
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func jsonStrings(points []HistoryPoint) string {
	parts := make([]string, len(points))
	for i, p := range points {
		parts[i] = fmt.Sprintf("%q", p.RunLabel)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// benchLabels returns a JSON array of short benchmark names for a package.
func benchLabels(benchmarks []BenchmarkHistory) string {
	parts := make([]string, len(benchmarks))
	for i, bh := range benchmarks {
		parts[i] = fmt.Sprintf("%q", bh.ShortName)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// benchLatestNs returns a JSON array of the latest ns/op for each benchmark.
func benchLatestNs(benchmarks []BenchmarkHistory) string {
	parts := make([]string, len(benchmarks))
	for i, bh := range benchmarks {
		pt := latestPoint(bh.Points)
		if pt.Present {
			parts[i] = fmt.Sprintf("%.2f", pt.NsPerOp)
		} else {
			parts[i] = "null"
		}
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// benchLatestAllocs returns a JSON array of the latest allocs/op for each benchmark.
func benchLatestAllocs(benchmarks []BenchmarkHistory) string {
	parts := make([]string, len(benchmarks))
	for i, bh := range benchmarks {
		pt := latestPoint(bh.Points)
		if pt.Present {
			parts[i] = fmt.Sprintf("%d", pt.AllocsPerOp)
		} else {
			parts[i] = "null"
		}
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// =============================================================================
// HTML Template
// =============================================================================

const reportTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Ghost MCP Benchmark Report</title>
<script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js" crossorigin="anonymous"></script>
<style>
  :root {
    --bg: #0f1117; --surface: #1a1d27; --surface2: #252836;
    --border: #2e3347; --text: #e2e8f0; --muted: #8892a4;
    --accent: #38bdf8; --green: #4ade80; --red: #f87171;
    --yellow: #fbbf24; --purple: #a78bfa;
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { background: var(--bg); color: var(--text); font-family: ui-monospace,'Cascadia Code','Fira Code',monospace; font-size: 14px; line-height: 1.6; }
  header { background: var(--surface); border-bottom: 1px solid var(--border); padding: 20px 32px; display: flex; align-items: center; gap: 16px; }
  header h1 { font-size: 20px; color: var(--accent); }
  header .meta { color: var(--muted); font-size: 12px; margin-left: auto; text-align: right; }
  main { max-width: 1400px; margin: 0 auto; padding: 24px 32px; }
  .section-title { font-size: 16px; color: var(--accent); margin: 32px 0 16px; border-bottom: 1px solid var(--border); padding-bottom: 8px; }
  .pkg-block { background: var(--surface); border: 1px solid var(--border); border-radius: 8px; margin-bottom: 32px; overflow: hidden; }
  .pkg-header { background: var(--surface2); padding: 12px 20px; display: flex; align-items: center; gap: 12px; }
  .pkg-name { font-size: 13px; color: var(--accent); }
  .pkg-body { padding: 20px; }
  .chart-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; margin-bottom: 24px; }
  @media (max-width: 900px) { .chart-grid { grid-template-columns: 1fr; } }
  .chart-box { background: var(--surface2); border-radius: 6px; padding: 16px; }
  .chart-box h3 { font-size: 11px; color: var(--muted); margin-bottom: 12px; text-transform: uppercase; letter-spacing: 0.05em; }
  canvas { max-height: 260px; }
  table { width: 100%; border-collapse: collapse; font-size: 13px; margin-top: 16px; }
  th { color: var(--muted); font-weight: 500; text-align: left; padding: 8px 12px; border-bottom: 1px solid var(--border); text-transform: uppercase; font-size: 11px; letter-spacing: 0.05em; }
  td { padding: 8px 12px; border-bottom: 1px solid var(--border); vertical-align: top; }
  tr:last-child td { border-bottom: none; }
  tr:hover td { background: var(--surface2); }
  .bench-name { color: var(--text); }
  .ns { color: var(--yellow); }
  .allocs { color: var(--purple); }
  .bytes { color: var(--accent); }
  .delta-faster { color: var(--green); }
  .delta-slower { color: var(--red); }
  .delta-same { color: var(--muted); }
  .runs-list { margin-bottom: 24px; }
  .run-item { background: var(--surface); border: 1px solid var(--border); border-radius: 6px; padding: 10px 16px; margin-bottom: 8px; display: flex; gap: 20px; flex-wrap: wrap; font-size: 12px; }
  .run-item .label { color: var(--muted); }
  .run-item .val { color: var(--text); }
  .empty { color: var(--muted); font-style: italic; padding: 16px 0; }
  .trend-btn { font-size: 11px; color: var(--accent); cursor: pointer; margin-left: 8px; background: none; border: none; padding: 0; }
  .trend-modal { display: none; position: fixed; inset: 0; background: rgba(0,0,0,0.7); z-index: 100; align-items: center; justify-content: center; }
  .trend-modal.open { display: flex; }
  .trend-box { background: var(--surface); border: 1px solid var(--border); border-radius: 8px; padding: 24px; max-width: 700px; width: 90%; }
  .trend-box h3 { color: var(--accent); margin-bottom: 16px; font-size: 14px; }
  .trend-close { float: right; cursor: pointer; color: var(--muted); font-size: 20px; line-height: 1; background: none; border: none; }
  .trend-close:hover { color: var(--text); }
</style>
</head>
<body>
<header>
  <div><h1>Ghost MCP — Benchmark Report</h1></div>
  <div class="meta">Generated: {{.GeneratedAt}}<br>{{len .Runs}} run(s) loaded</div>
</header>
<main>
{{if not .Runs}}
<p class="empty">No benchmark data found. Run <code>go run ./cmd/bench-report/</code> to generate results.</p>
{{else}}

<div class="section-title">Runs</div>
<div class="runs-list">
{{range .Runs}}
<div class="run-item">
  <span><span class="label">pkg </span><span class="val">{{.Package}}</span></span>
  <span><span class="label">branch </span><span class="val">{{.GitBranch}}</span></span>
  <span><span class="label">commit </span><span class="val">{{shortCommit .GitCommit}}</span></span>
  <span><span class="label">time </span><span class="val">{{.Timestamp}}</span></span>
  <span><span class="label">go </span><span class="val">{{.GoVersion}}</span></span>
  <span><span class="label">cpu </span><span class="val">{{.CPU}}</span></span>
</div>
{{end}}
</div>

{{range .Packages}}
<div class="pkg-block">
  <div class="pkg-header">
    <span class="pkg-name">{{.ShortPkg}}</span>
    <span style="color:var(--muted);font-size:12px;">{{len .Benchmarks}} benchmarks</span>
  </div>
  <div class="pkg-body">
  {{if .Benchmarks}}
    <div class="chart-grid">
      <div class="chart-box">
        <h3>Execution Time (ns/op) — Latest Run</h3>
        <canvas id="bar_{{safeID .Package}}"></canvas>
      </div>
      <div class="chart-box">
        <h3>Allocations (allocs/op) — Latest Run</h3>
        <canvas id="alloc_{{safeID .Package}}"></canvas>
      </div>
    </div>

    <table>
      <thead><tr>
        <th>Benchmark</th>
        <th>ns/op</th>
        <th>B/op</th>
        <th>allocs/op</th>
        <th>vs Previous</th>
      </tr></thead>
      <tbody>
      {{range .Benchmarks}}
      {{$pt := latestPoint .Points}}
      <tr>
        <td class="bench-name">{{.ShortName}}{{if gt (len .Points) 1}}<button class="trend-btn" onclick="openTrend('{{safeID .Name}}')">↗ trend</button>{{end}}</td>
        <td class="ns">{{if $pt.Present}}{{formatNs $pt.NsPerOp}}{{else}}&mdash;{{end}}</td>
        <td class="bytes">{{if $pt.Present}}{{$pt.BytesPerOp}}{{else}}&mdash;{{end}}</td>
        <td class="allocs">{{if $pt.Present}}{{$pt.AllocsPerOp}}{{else}}&mdash;{{end}}</td>
        <td>{{if .HasDelta}}{{if .Faster}}<span class="delta-faster">▼ {{printf "%.1f" (absf .Delta)}}% faster</span>{{else}}<span class="delta-slower">▲ {{printf "%.1f" (absf .Delta)}}% slower</span>{{end}}{{else}}<span class="delta-same">&mdash;</span>{{end}}</td>
      </tr>
      {{end}}
      </tbody>
    </table>

    {{range .Benchmarks}}{{if gt (len .Points) 1}}
    <div class="trend-modal" id="{{safeID .Name}}">
      <div class="trend-box">
        <button class="trend-close" onclick="closeTrend('{{safeID .Name}}')">&#x2715;</button>
        <h3>{{.ShortName}} — Trend</h3>
        <canvas id="tc_{{safeID .Name}}"></canvas>
      </div>
    </div>
    {{end}}{{end}}

  {{else}}
    <p class="empty">No benchmarks found for this package.</p>
  {{end}}
  </div>
</div>
{{end}}

{{end}}
</main>
<script>
Chart.defaults.color = '#8892a4';
Chart.defaults.borderColor = '#2e3347';
const COLORS = ['#38bdf8','#4ade80','#fbbf24','#f87171','#a78bfa','#34d399','#fb923c','#e879f9'];
function formatNs(ns) {
  if (ns === null || ns === undefined) return '-';
  if (ns < 1000) return ns.toFixed(1) + ' ns';
  if (ns < 1e6)  return (ns/1000).toFixed(2) + ' µs';
  if (ns < 1e9)  return (ns/1e6).toFixed(2) + ' ms';
  return (ns/1e9).toFixed(3) + ' s';
}
function openTrend(id)  { const el = document.getElementById(id); if(el) el.classList.add('open'); }
function closeTrend(id) { const el = document.getElementById(id); if(el) el.classList.remove('open'); }
document.addEventListener('click', e => {
  document.querySelectorAll('.trend-modal.open').forEach(m => { if (e.target === m) m.classList.remove('open'); });
});

{{range .Packages}}{{if .Benchmarks}}
(function(){
  const labels = {{benchLabels .Benchmarks}};
  const nsData  = {{benchLatestNs .Benchmarks}};
  const allocData = {{benchLatestAllocs .Benchmarks}};
  const mkColor = (a) => labels.map((_,i) => COLORS[i%COLORS.length] + a);
  new Chart(document.getElementById('bar_{{safeID .Package}}'),{
    type:'bar', data:{labels, datasets:[{label:'ns/op',data:nsData,backgroundColor:mkColor('cc'),borderColor:mkColor('ff'),borderWidth:1}]},
    options:{responsive:true,maintainAspectRatio:true,plugins:{legend:{display:false},tooltip:{callbacks:{label:c=>formatNs(c.raw)}}},scales:{y:{beginAtZero:true,ticks:{callback:v=>formatNs(v)}},x:{ticks:{maxRotation:45,font:{size:11}}}}}
  });
  new Chart(document.getElementById('alloc_{{safeID .Package}}'),{
    type:'bar', data:{labels, datasets:[{label:'allocs/op',data:allocData,backgroundColor:'#a78bfacc',borderColor:'#a78bfa',borderWidth:1}]},
    options:{responsive:true,maintainAspectRatio:true,plugins:{legend:{display:false}},scales:{y:{beginAtZero:true},x:{ticks:{maxRotation:45,font:{size:11}}}}}
  });
{{range .Benchmarks}}{{if gt (len .Points) 1}}
  new Chart(document.getElementById('tc_{{safeID .Name}}'),{
    type:'line', data:{labels:{{jsonStrings .Points}},datasets:[{label:'ns/op',data:{{jsonNumbers .Points}},borderColor:'#38bdf8',backgroundColor:'#38bdf820',tension:0.3,fill:true,pointRadius:4,spanGaps:true}]},
    options:{responsive:true,plugins:{legend:{display:false},tooltip:{callbacks:{label:c=>formatNs(c.raw)}}},scales:{y:{ticks:{callback:v=>formatNs(v)}}}}
  });
{{end}}{{end}}
})();
{{end}}{{end}}
</script>
</body>
</html>`
