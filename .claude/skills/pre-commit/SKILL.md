# /pre-commit

Run all local CI gate checks before a commit and fix any issues found.
This skill mirrors the CI pipeline (lint.yml + test.yml) so failures are caught locally.

## When to invoke
- Before `git commit` — automatically via the git hook, or manually.
- After editing Go source, CRD types, or API handler files.

## Hook installation (one-time per clone)
```sh
ln -sf ../../hack/pre-commit.sh .git/hooks/pre-commit
```

## What the gate runs

### 1. go fmt
```sh
gofmt -l ./...
```
Fail: any file listed → run `go fmt ./...` and re-stage.

**Fallback — `gofmt -l ./...` returns `stat ./...: no such file or directory`:**

This happens when the shell's working directory is not the repo root (common in agent/tool environments). Use `find` + explicit file list instead:

```sh
# Check
gofmt -l $(find . -name '*.go')

# Fix
gofmt -w $(find . -name '*.go')
```

Always run these commands from the repo root (`/Users/mirac.yilmaz/Desktop/mirac/korsair-operator`).

### 2. go vet
```sh
go vet ./...
```
Fail: fix the reported issue before proceeding.

### 3. golangci-lint
```sh
golangci-lint run ./...
```
Config: `.golangci.yml` — enabled linters: errcheck, goconst, lll (120 chars), modernize, staticcheck, revive, and others.

**Common fixes:**
| Linter | Pattern | Fix |
|--------|---------|-----|
| `errcheck` | `defer f.Close()` | `defer func() { _ = f.Close() }()` |
| `errcheck` | `defer tx.Rollback()` | `defer func() { _ = tx.Rollback() }()` |
| `errcheck` | `resp.Body.Close()` | `_ = resp.Body.Close()` |
| `goconst` | string literal ≥3 times | extract to `const` in the same package |
| `lll` | line > 120 chars | break into multiple lines |
| `modernize` | `for i := 0; i < N; i++` | `for range N` |
| `staticcheck S1016` | struct literal copy | direct type conversion `T2(v)` |

If golangci-lint is not installed locally:
```sh
# Install (macOS)
brew install golangci-lint
# Or via script
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v2.8.0
```

### 4. Unit tests
```sh
KUBEBUILDER_ASSETS=$(./bin/setup-envtest use 1.35 -p path) \
  go test ./internal/... ./cmd/... -v
```
Expected: **40/40 specs pass** in `internal/controller`.

### 5. Repo validation
Automated checks that catch open-source contract violations:
- No proprietary references (`company_name`, `INTERNAL_URL`) in Go source.
- Every `*_types.go` file has `// +kubebuilder:resource` marker.
- CSV schema header in `cmd/web/main.go` is unchanged (public contract).
- `imageToJobName()` signature still includes `digest` parameter (deduplication invariant).

## Running manually
```sh
./hack/pre-commit.sh
```

## CI equivalence
| Local step | CI workflow |
|------------|-------------|
| go fmt + go vet + golangci-lint | `.github/workflows/lint.yml` (golangci-lint v2.8.0) |
| go test ./internal/... ./cmd/... | `.github/workflows/test.yml` (make test) |
| Repo validation | — (static, no CI equivalent) |

Fix all local failures before pushing — the same checks will block the CI run.
