# Rebuilding Windows Dependency Bundle

## Overview

The CI uses a pre-built dependency bundle to avoid the ~20 minute vcpkg build on every CI run. This bundle contains:

- **Tesseract OCR** (now 5.5.2)
- **Leptonica** (image processing library)
- **MinGW-w64 runtime DLLs**
- **Tesseract language data** (eng.traineddata)

## When to Rebuild

Rebuild the Windows dependency bundle when:
- ✅ Tesseract version needs updating (e.g., 5.5.0 → 5.5.2)
- ✅ Leptonica version needs updating
- ✅ The x64-mingw-dynamic triplet changes
- ✅ The bundle release doesn't exist yet

## Steps to Rebuild

### 1. Go to GitHub Actions

Navigate to: **https://github.com/ghost-mcp/ghost-mcp/actions/workflows/build-deps-windows.yml**

### 2. Run the Workflow

1. Click **"Run workflow"** button
2. Select branch (usually `main`)
3. Enter a **new tag**: `deps-windows-v2`
4. Click **"Run workflow"**

### 3. Wait for Completion

The workflow will:
1. Install MinGW via Chocolatey (~2 min)
2. Clone and bootstrap vcpkg (~1 min)
3. **Build Tesseract + Leptonica** (~20-25 min)
4. Create symlinks for gosseract (~1 min)
5. Download Tesseract language data (~30 sec)
6. Package into ZIP bundle (~1 min)
7. Create GitHub Release (~30 sec)

**Total time: ~25-30 minutes**

### 4. Verify the Release

After completion, verify the release exists:
- Go to: **https://github.com/ghost-mcp/ghost-mcp/releases**
- Look for: `deps-windows-v2`
- Should contain: `deps-windows-x64-mingw-dynamic.zip` (~100-150 MB)

### 5. Update test.yml

Update the workflow to use the new version:

```yaml
# In .github/workflows/test.yml
gh release download deps-windows-v2 `  # ← Update from v1 to v2
```

Also update the cache key:

```yaml
# In .github/workflows/test.yml
key: vcpkg-x64-mingw-tesseract-leptonica-v2  # ← Increment version
```

## Current Versions (as of 2026-04-02)

| Package | Version | Notes |
|---------|---------|-------|
| **Go** | 1.26.0 | Latest stable |
| **Tesseract** | 5.5.2 | December 2025 release |
| **Leptonica** | 1.87.0 | Bundled with Tesseract 5.5.2 |
| **robotgo** | 1.0.2 | Latest |
| **gosseract** | 2.4.1 | Latest |

## Troubleshooting

### Workflow Fails

If the workflow fails:
1. Check the logs for specific errors
2. Common issues:
   - Network timeout during vcpkg build → Re-run workflow
   - GitHub API rate limit → Wait and retry
   - Disk space → Runner should have sufficient space

### Bundle Too Large

If the bundle exceeds GitHub's 2GB release limit:
- This is unlikely (~100-150 MB expected)
- Consider splitting into multiple archives if needed

### Tests Still Use Old Version

If CI tests still show old Tesseract version:
1. Verify `test.yml` downloads `deps-windows-v2` (not v1)
2. Clear the vcpkg cache if needed
3. Force re-download by deleting the release and re-running

## Local Testing

To test the new bundle locally before committing:

```powershell
# Download the release asset
gh release download deps-windows-v2 `
  --repo ghost-mcp/ghost-mcp `
  --pattern "deps-windows-x64-mingw-dynamic.zip" `
  --dir "$env:TEMP"

# Extract to vcpkg installed directory
Expand-Archive "$env:TEMP\deps-windows-x64-mingw-dynamic.zip" `
  -DestinationPath "$env:USERPROFILE\vcpkg\installed\x64-mingw-dynamic" `
  -Force

# Verify Tesseract version
& "$env:USERPROFILE\vcpkg\packages\tesseract_x64-mingw-dynamic\tools\tesseract\tesseract.exe" --version
# Should show: tesseract 5.5.2

# Run tests
go test -v -run TestAccuracy ./cmd/ghost-mcp/...
```

## Cost Optimization

The pre-built bundle saves significant CI time:
- **Without bundle**: ~25 min vcpkg build per Windows CI run
- **With bundle**: ~30 sec download and extract
- **Savings**: ~24.5 minutes per Windows CI job
- **Cost savings**: Significant for frequent CI runs

## References

- Workflow file: `.github/workflows/build-deps-windows.yml`
- Test workflow: `.github/workflows/test.yml`
- vcpkg triplet: `x64-mingw-dynamic`
- Tesseract releases: https://github.com/tesseract-ocr/tesseract/releases
