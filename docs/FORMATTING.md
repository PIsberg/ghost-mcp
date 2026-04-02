# Code Formatting and Pre-commit Hooks

## Problem
Go requires properly formatted code. Unformatted code causes CI failures and makes code reviews harder.

## Solution: Automated Formatting

### Option 1: Pre-commit Hook (Recommended) ✅

A pre-commit hook automatically formats your code before every commit.

#### Installation

**Windows (Git Bash):**
```bash
# The hook is already installed at .git/hooks/pre-commit
# Make it executable (if using Git Bash)
chmod +x .git/hooks/pre-commit
```

**Windows (CMD/PowerShell):**
```powershell
# The batch file is already installed at .git\hooks\pre-commit.bat
# Git will automatically use it on Windows
```

**Manual installation:**
```bash
# Copy the hook template
cp .git/hooks/pre-commit.sample .git/hooks/pre-commit 2>/dev/null || true
cp scripts/pre-commit .git/hooks/pre-commit 2>/dev/null || true

# Make executable
chmod +x .git/hooks/pre-commit
```

#### What It Does

The pre-commit hook automatically:
1. ✅ **Formats Go files** with `gofmt -w`
2. ✅ **Runs `go vet`** to catch common errors
3. ✅ **Runs `go mod tidy`** to clean up dependencies
4. ✅ **Blocks commit** if `go vet` fails

#### Example Output

```
Running pre-commit checks...
📝 Formatting Go files...
⚠️  The following files need formatting:
cmd/ghost-mcp/handler_ocr.go
internal/ocr/ocr.go

📝 Auto-formatting...
✅ Files formatted. Please review and commit again.

💡 Tip: Run 'gofmt -w .' before committing to avoid this message
🔍 Running go vet...
✅ go vet passed
📦 Checking Go modules...
✅ Go modules are tidy

✅ All pre-commit checks passed!
```

---

### Option 2: Manual Formatting

Run before committing:

```bash
# Format all Go files
gofmt -w .

# Verify formatting
gofmt -l .

# Run go vet
go vet ./...

# Tidy modules
go mod tidy
```

---

### Option 3: Pre-commit Framework

If you use [pre-commit](https://pre-commit.com/):

```bash
# Install pre-commit
pip install pre-commit

# Install hooks
pre-commit install

# Run manually
pre-commit run
```

The `.pre-commit-config.yaml` is already configured with:
- ✅ gofmt (auto-format Go files)
- ✅ golangci-lint (comprehensive linting)
- ✅ gitleaks (secret detection)
- ✅ shellcheck (shell script linting)
- ✅ end-of-file-fixer
- ✅ trailing-whitespace

---

### Option 4: IDE Integration

Most Go IDEs can format on save:

#### VS Code
```json
{
  "go.formatTool": "gofmt",
  "go.formatOnSave": true,
  "go.lintOnSave": "package",
  "go.vetOnSave": "package"
}
```

#### GoLand/IntelliJ
- Settings → Go → Goimports/gofmt
- ✅ "Run gofmt/gofumpt on save"
- ✅ "Optimize imports on save"

#### Vim/Neovim
```vim
" In .vimrc or init.vim
autocmd BufWritePre *.go execute ':silent! !gofmt -w %'
autocmd BufWritePre *.go execute ':silent! :edit!'
```

---

## CI/CD Integration

### GitHub Actions

The CI already checks formatting:

```yaml
# .github/workflows/test.yml
- name: Check Go formatting
  run: |
    UNFORMATTED=$(gofmt -l .)
    if [ -n "$UNFORMATTED" ]; then
      echo "❌ The following files are not formatted:"
      echo "$UNFORMATTED"
      exit 1
    fi
```

### Local CI Check

Before pushing, run:

```bash
# Check if CI will pass
gofmt -l . && go vet ./... && echo "✅ Ready to push"
```

---

## Troubleshooting

### "pre-commit hook failed"

**Problem:** Hook blocked your commit

**Solution:**
1. Read the error message
2. Fix the issue (e.g., `go vet` failure)
3. Commit again

### "gofmt not found"

**Problem:** Go is not in PATH

**Solution:**
```bash
# Add Go to PATH
export PATH=$PATH:/usr/local/go/bin  # Linux/macOS
setx PATH "%PATH%;C:\Program Files\Go\bin"  # Windows (permanent)
```

### "Hook not running"

**Problem:** Hook is not executable

**Solution:**
```bash
# Make hook executable
chmod +x .git/hooks/pre-commit

# Or on Windows, ensure .bat file exists
copy .git\hooks\pre-commit.bat .git\hooks\pre-commit 2>nul
```

### "Want to bypass hook"

**Problem:** Need to commit without running hooks

**Solution:**
```bash
# Use --no-verify to skip hooks
git commit -m "message" --no-verify

# ⚠️ Only use for emergencies - CI will still check!
```

---

## Best Practices

1. ✅ **Always run pre-commit hooks** - They catch issues early
2. ✅ **Format before committing** - `gofmt -w .`
3. ✅ **Run go vet** - Catches common errors
4. ✅ **Keep modules tidy** - `go mod tidy`
5. ✅ **Use IDE auto-format** - Format on save

---

## Files

| File | Purpose |
|------|---------|
| `.git/hooks/pre-commit` | Shell script hook (Linux/macOS/Git Bash) |
| `.git/hooks/pre-commit.bat` | Batch file hook (Windows CMD) |
| `.pre-commit-config.yaml` | Pre-commit framework config |
| `docs/FORMATTING.md` | This documentation |

---

## Quick Reference

```bash
# Format code
gofmt -w .

# Check formatting (CI check)
gofmt -l .

# Run all checks
go vet ./... && go mod tidy && gofmt -l .

# Install pre-commit hook
pre-commit install

# Run hooks manually
pre-commit run

# Skip hooks (emergency only)
git commit --no-verify
```

---

## See Also

- [Go fmt documentation](https://pkg.go.dev/cmd/gofmt)
- [Pre-commit framework](https://pre-commit.com/)
- [Go vet documentation](https://pkg.go.dev/cmd/vet)
