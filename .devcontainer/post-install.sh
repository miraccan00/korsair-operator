#!/usr/bin/env bash
# .devcontainer/post-install.sh
#
# Runs once after the devcontainer is created (postCreateCommand).
# All CLI tools (kubectl, kind, tilt, helm, kubebuilder) are already installed
# in the Dockerfile image layer. This script only handles tasks that require a
# live Docker socket or the project workspace to be mounted:
#
#   1. Wait for the Docker socket to be ready
#   2. Ensure the `kind` Docker network exists
#   3. Pre-download Go module dependencies
#   4. Install envtest binaries for `make test`
#   5. Install Go dev toolchain (controller-gen, golangci-lint, etc.)
#   6. pnpm (frontend toolchain)
#
# NOTE: Kind cluster creation and kubeconfig wiring are handled by Tiltfile-local.
#       Run `tilt up -f Tiltfile-local` after the devcontainer is ready.

set -euo pipefail

WORKSPACE="${WORKSPACE:-/workspaces/korsair-operator}"

# ── 1. Wait for Docker socket ──────────────────────────────────────────────────
echo "==> Waiting for Docker daemon..."
for i in $(seq 1 30); do
  if docker info >/dev/null 2>&1; then
    echo "    Docker ready (attempt ${i})"
    break
  fi
  if [ "${i}" -eq 30 ]; then
    echo "WARNING: Docker not ready after 30s — continuing anyway"
  fi
  sleep 1
done

# ── 2. kind Docker network ─────────────────────────────────────────────────────
echo "==> Ensuring 'kind' Docker network exists..."
docker network inspect kind >/dev/null 2>&1 \
  || docker network create kind \
  && echo "    Created 'kind' network" \
  || echo "    'kind' network already exists"

# ── 3. Go module cache ─────────────────────────────────────────────────────────
echo "==> Pre-downloading Go modules..."
cd "${WORKSPACE}"
go mod download

# ── 4. envtest binaries (for make test) ───────────────────────────────────────
echo "==> Installing envtest binaries..."
make setup-envtest

# ── 5. Go dev toolchain ────────────────────────────────────────────────────────
echo "==> Installing Go dev tools..."
make controller-gen kustomize golangci-lint

# ── 6. pnpm (frontend toolchain) ──────────────────────────────────────────────
# Node.js is installed via the devcontainer feature (see devcontainer.json).
# corepack is bundled with Node 22 and manages pnpm versions.
echo "==> Enabling pnpm via corepack..."
corepack enable pnpm

echo ""
echo "============================================================"
echo "  DevContainer ready!"
echo ""
echo "  Full-stack dev (Tilt):    tilt up -f Tiltfile-local"
echo "  Frontend-only dev:        docker compose up -d && pnpm -C frontend dev"
echo "  Run tests:                make test"
echo "============================================================"
