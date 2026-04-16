#!/usr/bin/env bash
# hack/pre-commit.sh — local pre-commit gate that mirrors CI checks.
#
# Install as a git hook (one-time, per clone):
#   ln -sf ../../hack/pre-commit.sh .git/hooks/pre-commit
#
# Or run manually before committing:
#   ./hack/pre-commit.sh
#
# Exit codes: 0 = all checks passed, 1 = at least one check failed.
set -euo pipefail

BOLD='\033[1m'
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
RESET='\033[0m'

FAILED=0

pass() { echo -e "  ${GREEN}✓${RESET} $1"; }
fail() { echo -e "  ${RED}✗${RESET} $1"; FAILED=1; }
header() { echo -e "\n${BOLD}$1${RESET}"; }

# ── 1. Format ─────────────────────────────────────────────────────────────────
header "1/5  go fmt"
UNFORMATTED=$(gofmt -l ./... 2>/dev/null | grep -v vendor || true)
if [ -n "$UNFORMATTED" ]; then
  fail "unformatted files (run: go fmt ./...):"
  echo "$UNFORMATTED" | sed 's/^/       /'
else
  pass "all files formatted"
fi

# ── 2. Vet ────────────────────────────────────────────────────────────────────
header "2/5  go vet"
if go vet ./... 2>&1; then
  pass "go vet clean"
else
  fail "go vet found issues"
fi

# ── 3. Lint ───────────────────────────────────────────────────────────────────
header "3/5  golangci-lint"
if command -v golangci-lint &>/dev/null; then
  if golangci-lint run ./... 2>&1; then
    pass "golangci-lint clean"
  else
    fail "golangci-lint found issues (same checks as CI lint.yml)"
  fi
else
  echo -e "  ${YELLOW}⚠${RESET}  golangci-lint not installed — skipping"
  echo "       Install: https://golangci-lint.run/welcome/install/"
fi

# ── 4. Unit tests ─────────────────────────────────────────────────────────────
header "4/5  unit tests"
KUBEBUILDER_ASSETS=$(./bin/setup-envtest use 1.35 -p path 2>/dev/null \
  || echo "./bin/k8s/1.35.0-$(go env GOOS)-$(go env GOARCH)")
export KUBEBUILDER_ASSETS
if KUBEBUILDER_ASSETS="$KUBEBUILDER_ASSETS" go test ./internal/... ./cmd/... 2>&1; then
  pass "all tests passed"
else
  fail "tests failed"
fi

# ── 5. Repo validation ────────────────────────────────────────────────────────
header "5/5  repo validation"

# 5a. No proprietary/internal references in Go source
INTERNAL_REFS=$(grep -rn --include="*.go" \
  "company_name\|internal-only\|INTERNAL_URL" \
  --exclude-dir=vendor . 2>/dev/null || true)
if [ -n "$INTERNAL_REFS" ]; then
  fail "proprietary references found in Go source:"
  echo "$INTERNAL_REFS" | sed 's/^/       /'
else
  pass "no proprietary references"
fi

# 5b. CRD markers present in every types file
TYPES_WITHOUT_MARKERS=$(grep -rL "kubebuilder:resource" api/v1alpha1/*_types.go 2>/dev/null || true)
if [ -n "$TYPES_WITHOUT_MARKERS" ]; then
  fail "types files missing kubebuilder:resource markers:"
  echo "$TYPES_WITHOUT_MARKERS" | sed 's/^/       /'
else
  pass "kubebuilder markers present"
fi

# 5c. CSV schema header constant — must not have been changed
EXPECTED_HEADER='"Image", "Target", "Library", "VulnerabilityID", "Severity", "Status", "InstalledVersion", "FixedVersion", "Title"'
if grep -qF "$EXPECTED_HEADER" cmd/web/main.go 2>/dev/null; then
  pass "CSV schema header unchanged"
else
  echo -e "  ${YELLOW}⚠${RESET}  CSV schema header may have changed — verify it is backward-compatible"
fi

# 5d. imageToJobName uses digest parameter (deduplication invariant)
if grep -q "func imageToJobName" internal/controller/*.go 2>/dev/null; then
  if grep -A2 "func imageToJobName" internal/controller/*.go | grep -q "digest"; then
    pass "imageToJobName includes digest parameter"
  else
    fail "imageToJobName missing digest parameter — digest deduplication is broken"
  fi
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
if [ "$FAILED" -eq 0 ]; then
  echo -e "${GREEN}${BOLD}All checks passed — safe to commit.${RESET}"
else
  echo -e "${RED}${BOLD}Pre-commit checks failed — fix issues above before committing.${RESET}"
  exit 1
fi
