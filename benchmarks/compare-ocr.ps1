param(
    [string]$BaselineRef = "origin/main",
    [int]$Count = 5,
    [string]$OutputDir = "benchmarks/results"
)

$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$outputRoot = Join-Path $repoRoot $OutputDir
$historyDir = Join-Path $outputRoot "history"
New-Item -ItemType Directory -Force -Path $outputRoot | Out-Null
New-Item -ItemType Directory -Force -Path $historyDir | Out-Null

$benchFiles = @(
    "internal/ocr/benchmark_fixture_test.go",
    "cmd/ghost-mcp/benchmark_fixture_test.go"
)
$currentPackages = @("./internal/ocr", "./cmd/ghost-mcp")
$baselinePackages = @("./internal/ocr")
$benchRegex = "Benchmark(ReadImage_FixturePanel|ParallelFindText_FixtureButtons)"

$env:PATH = "$env:USERPROFILE\vcpkg\installed\x64-mingw-dynamic\bin;$env:PATH"
$env:TESSDATA_PREFIX = "$env:USERPROFILE\vcpkg\installed\x64-mingw-dynamic\share\tessdata"

function Get-GitInfo {
    param([string]$Workdir, [string]$Ref)

    $commit = (git -C $Workdir rev-parse $Ref).Trim()
    $short = (git -C $Workdir rev-parse --short $Ref).Trim()
    $branch = (git -C $Workdir rev-parse --abbrev-ref $Ref).Trim()

    return @{
        ref = $Ref
        commit = $commit
        short_commit = $short
        branch = $branch
    }
}

function Copy-BenchmarkOverlay {
    param([string]$SourceRoot, [string]$TargetRoot)

    foreach ($rel in $benchFiles) {
        $src = Join-Path $SourceRoot $rel
        $dst = Join-Path $TargetRoot $rel
        $dstDir = Split-Path -Parent $dst
        New-Item -ItemType Directory -Force -Path $dstDir | Out-Null
        Copy-Item $src $dst -Force
    }
}

function Parse-BenchmarkRuns {
    param([string[]]$Lines)

    $results = @{}
    foreach ($line in $Lines) {
        if ($line -match '^(Benchmark\S+?)(?:-\d+)?\s+\d+\s+([0-9.]+)\s+ns/op(?:\s+([0-9.]+)\s+B/op\s+([0-9.]+)\s+allocs/op)?') {
            $name = $matches[1]
            if (-not $results.ContainsKey($name)) {
                $results[$name] = [System.Collections.Generic.List[object]]::new()
            }

            $entry = [ordered]@{
                ns_per_op = [double]$matches[2]
            }
            if ($matches[3]) {
                $entry.bytes_per_op = [double]$matches[3]
            }
            if ($matches[4]) {
                $entry.allocs_per_op = [double]$matches[4]
            }
            $results[$name].Add($entry)
        }
    }
    return $results
}

function Get-Median {
    param([double[]]$Values)

    $sorted = $Values | Sort-Object
    $count = $sorted.Count
    if ($count -eq 0) {
        return $null
    }
    if ($count % 2 -eq 1) {
        return [double]$sorted[[int]($count / 2)]
    }
    return ([double]$sorted[$count / 2 - 1] + [double]$sorted[$count / 2]) / 2.0
}

function Summarize-Benchmarks {
    param([hashtable]$Runs)

    $summary = [ordered]@{}
    foreach ($name in $Runs.Keys | Sort-Object) {
        $entries = $Runs[$name]
        $summary[$name] = [ordered]@{
            runs = @($entries)
            median_ns_per_op = Get-Median (($entries | ForEach-Object { $_.ns_per_op }))
        }

        $bytes = @($entries | Where-Object { $_.Contains("bytes_per_op") } | ForEach-Object { $_.bytes_per_op })
        if ($bytes.Count -gt 0) {
            $summary[$name].median_bytes_per_op = Get-Median $bytes
        }

        $allocs = @($entries | Where-Object { $_.Contains("allocs_per_op") } | ForEach-Object { $_.allocs_per_op })
        if ($allocs.Count -gt 0) {
            $summary[$name].median_allocs_per_op = Get-Median $allocs
        }
    }
    return $summary
}

function Invoke-BenchmarkSet {
    param(
        [string]$Workdir,
        [string]$Label,
        [hashtable]$GitInfo,
        [string[]]$Packages
    )

    $rawPath = Join-Path $historyDir ("{0}-{1}.txt" -f $Label, $GitInfo.short_commit)
    $jsonPath = Join-Path $historyDir ("{0}-{1}.json" -f $Label, $GitInfo.short_commit)

    $cmd = "go test -run '^$' -bench '$benchRegex' -benchmem -count $Count $($Packages -join ' ')"
    $output = & pwsh -NoLogo -NoProfile -Command $cmd 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "Benchmark command failed for $Label"
    }

    Set-Content -Path $rawPath -Value $output

    $runs = Parse-BenchmarkRuns $output
    $summary = Summarize-Benchmarks $runs

    $payload = [ordered]@{
        generated_at = (Get-Date).ToString("s")
        label = $Label
        git = $GitInfo
        benchmark_regex = $benchRegex
        count = $Count
        benchmarks = $summary
        raw_output = [System.IO.Path]::GetRelativePath($repoRoot, $rawPath)
    }

    $payload | ConvertTo-Json -Depth 8 | Set-Content -Path $jsonPath
    return @{
        json_path = $jsonPath
        raw_path = $rawPath
        payload = $payload
    }
}

function Compare-Benchmarks {
    param(
        [hashtable]$CurrentPayload,
        [hashtable]$BaselinePayload
    )

    $names = @($CurrentPayload.benchmarks.Keys + $BaselinePayload.benchmarks.Keys | Sort-Object -Unique)
    $comparisons = [ordered]@{}
    foreach ($name in $names) {
        $curr = $CurrentPayload.benchmarks[$name]
        $base = $BaselinePayload.benchmarks[$name]
        if (-not $curr -or -not $base) {
            continue
        }
        $comparisons[$name] = [ordered]@{
            current_median_ns_per_op = $curr.median_ns_per_op
            baseline_median_ns_per_op = $base.median_ns_per_op
            delta_ns_per_op = [double]$curr.median_ns_per_op - [double]$base.median_ns_per_op
            delta_pct = (([double]$curr.median_ns_per_op - [double]$base.median_ns_per_op) / [double]$base.median_ns_per_op) * 100.0
        }

        if ($curr.median_bytes_per_op -and $base.median_bytes_per_op) {
            $comparisons[$name].current_median_bytes_per_op = $curr.median_bytes_per_op
            $comparisons[$name].baseline_median_bytes_per_op = $base.median_bytes_per_op
            $comparisons[$name].delta_bytes_per_op = [double]$curr.median_bytes_per_op - [double]$base.median_bytes_per_op
        }

        if ($curr.median_allocs_per_op -and $base.median_allocs_per_op) {
            $comparisons[$name].current_median_allocs_per_op = $curr.median_allocs_per_op
            $comparisons[$name].baseline_median_allocs_per_op = $base.median_allocs_per_op
            $comparisons[$name].delta_allocs_per_op = [double]$curr.median_allocs_per_op - [double]$base.median_allocs_per_op
        }
    }
    return $comparisons
}

$currentInfo = Get-GitInfo -Workdir $repoRoot -Ref "HEAD"
$baselineInfo = Get-GitInfo -Workdir $repoRoot -Ref $BaselineRef

$baselineWorktree = Join-Path $outputRoot ("_baseline-{0}" -f $baselineInfo.short_commit)
if (Test-Path $baselineWorktree) {
    git -C $repoRoot worktree remove $baselineWorktree --force | Out-Null
}

git -C $repoRoot worktree add --detach $baselineWorktree $baselineInfo.commit | Out-Null
try {
    Copy-BenchmarkOverlay -SourceRoot $repoRoot -TargetRoot $baselineWorktree

    Push-Location $repoRoot
    $currentRun = Invoke-BenchmarkSet -Workdir $repoRoot -Label "current" -GitInfo $currentInfo -Packages $currentPackages
    Pop-Location

    Push-Location $baselineWorktree
    $baselineRun = Invoke-BenchmarkSet -Workdir $baselineWorktree -Label "baseline" -GitInfo $baselineInfo -Packages $baselinePackages
    Pop-Location

    $comparison = [ordered]@{
        generated_at = (Get-Date).ToString("s")
        current = $currentRun.payload.git
        baseline = $baselineRun.payload.git
        benchmarks = Compare-Benchmarks -CurrentPayload $currentRun.payload -BaselinePayload $baselineRun.payload
        current_result = [System.IO.Path]::GetRelativePath($repoRoot, $currentRun.json_path)
        baseline_result = [System.IO.Path]::GetRelativePath($repoRoot, $baselineRun.json_path)
    }

    $comparisonPath = Join-Path $outputRoot ("compare-{0}-vs-{1}.json" -f $currentInfo.short_commit, $baselineInfo.short_commit)
    $comparison | ConvertTo-Json -Depth 8 | Set-Content -Path $comparisonPath

    Copy-Item $baselineRun.json_path (Join-Path $outputRoot "latest-baseline.json") -Force
    Copy-Item $currentRun.json_path (Join-Path $outputRoot "latest-current.json") -Force
    Copy-Item $comparisonPath (Join-Path $outputRoot "latest-comparison.json") -Force

    Write-Host "Current results:   $([System.IO.Path]::GetRelativePath($repoRoot, $currentRun.json_path))"
    Write-Host "Baseline results:  $([System.IO.Path]::GetRelativePath($repoRoot, $baselineRun.json_path))"
    Write-Host "Comparison report: $([System.IO.Path]::GetRelativePath($repoRoot, $comparisonPath))"
}
finally {
    Pop-Location -ErrorAction SilentlyContinue
    git -C $repoRoot worktree remove $baselineWorktree --force | Out-Null
}
