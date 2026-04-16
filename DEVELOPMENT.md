# Korsair Operator — Development Guide

Welcome! This guide helps you set up a local development environment for Korsair Operator.

## Quick Start (3 Options)

### Option 1: Tilt (⭐ Recommended for Quick Testing)

**Best for:** Integrated logs, tests, and status monitoring via web UI.

```bash
# Prerequisites: Docker, kubectl, kind, tilt
brew install tilt  # macOS with Homebrew

# 1. Create kind cluster first
kind create cluster --name korsair-dev --image kindest/node:v1.32.0

# 2. Start Tilt (uses DockerHub images by default)
make tilt-up

# Opens http://localhost:10350 in your browser
```

**Mode 1: DockerHub Images (Default - Fast)**
- Pulls `miraccan/korsair-operator:latest` from DockerHub
- No local build, instant deployment
- Best for testing without code changes

**Mode 2: Local Build (Optional - Live Reload)**
- Uncomment `docker_build()` in Tiltfile
- Local code changes → auto rebuild → re-deploy
- Slower but useful for development
- See Tiltfile comments for setup

**Workflow in Tilt Web UI:**
1. Click `[setup]` → `k8s-ready` to verify/create kind cluster
2. Click `[deploy]` → `helm-install` to deploy operator
3. Click `[tests]` → `smoke-test` to verify deployment
4. Click `[debug]` → `operator-logs`, `operator-status` for monitoring
5. Click `[cleanup]` → `cleanup` to delete cluster

**Status colors:**
- 🟢 Green: Ready
- 🟡 Yellow: Running/Pending
- 🔴 Red: Failed/Error

**View logs from CLI:**
```bash
make tilt-logs
```

**Check status:**
```bash
make tilt-debug
```

**Stop:**
```bash
make tilt-down
```

---

### Option 2: Fast-Up (One-Shot Deployment)

**Best for:** Quick test without live reload.

```bash
# Creates kind cluster, builds image, deploys via Helm, runs smoke test
make fast-up

# Check operator status
kubectl -n korsair-system get pods
kubectl -n korsair-system logs -f -l app.kubernetes.io/component=operator

# Cleanup
make fast-down
```

---

### Option 3: Manual Setup

**Best for:** Understanding each step.

```bash
# 1. Create kind cluster
kind create cluster --name korsair-dev --image kindest/node:v1.32.0

# 2. Switch context
kubectl config use-context kind-korsair-dev

# 3. Build & load image locally
docker build -t controller:latest .
kind load docker-image controller:latest --name korsair-dev

# 4. Install via Helm
helm install korsair ./charts/korsair \
  --create-namespace \
  --namespace korsair-system \
  --set operator.image.repository=controller \
  --set operator.image.tag=latest \
  --set operator.image.pullPolicy=Never

# 5. Verify deployment
kubectl wait --for=condition=ready pod \
  -l app.kubernetes.io/component=operator \
  -n korsair-system --timeout=2m

# 6. Run smoke test
kubectl apply -f config/samples/security_v1alpha1_securityscanconfig.yaml
kubectl wait --for=condition=ready imagescanjob -A --timeout=60s
```

---

## Local Development Workflow

### With Tilt (Recommended)

```bash
# Terminal 1: Start Tilt
make tilt-up
# Web UI at http://localhost:10350

# Terminal 2: Edit code
vim internal/controller/securityscanconfig_reconciler.go

# Terminal 3: Watch operator logs
make tilt-logs
```

**Automatic flow:**
1. Code saved → Docker image rebuilt
2. Image loaded into kind cluster
3. Deployment updated
4. Logs streamed to Tilt UI
5. Tests can be triggered from UI

### Without Tilt

```bash
# Build operator locally (no cluster)
make build

# Run operator against current kubeconfig
make run

# In another terminal, test the operator
kubectl apply -f config/samples/security_v1alpha1_securityscanconfig.yaml
kubectl get imagescanjobs -w
```

---

## Image Optimization

### Build Context

Docker uses a **single `.dockerignore` file** for all builds, regardless of which Dockerfile is used:

- **Dockerfile** → uses `.dockerignore`
- **Dockerfile.api** → uses `.dockerignore` 
- **Dockerfile.ui** → uses `.dockerignore`

(Docker does NOT support per-Dockerfile `.dockerignore` files like `.dockerignore.api`)

Our `.dockerignore` is optimized to:
- Exclude heavy dependencies (frontend/node_modules, go.mod downloading)
- Skip test artifacts, docs, CI configs
- Keep context size ~1.8MB (vs ~500MB unfiltered)

### BuildKit Cache

**Local development (Tilt):**
Tiltfile uses fine-grained context filtering:
```yaml
docker_build(
    ref=OPERATOR_IMAGE,
    only=[                        # Only these files → context ~100KB
        "./api/",
        "./cmd/main.go",
        "./internal/",
        "./go.mod",
        "./go.sum",
    ],
)
```

This is **more efficient than `.dockerignore`** because:
- `.dockerignore` excludes files (build context still transfers them, then filters)
- Tiltfile's `only=[]` doesn't even transfer excluded files

**For direct docker builds:**
```bash
# Build with cache mount (2x faster on 2nd+ builds)
make docker-build-cached

# Or build all 3 images in parallel
make docker-build-all
```

**Cache mounts:** Go modules and build artifacts are cached in layers, so:
- 1st build: normal speed (context: ~1.8MB)
- 2nd+ builds: 3-5x faster (cache hit)

---

## Testing

### Unit Tests

```bash
make test
```

### Lint

```bash
make lint          # Check
make lint-fix      # Auto-fix issues
```

### E2E Tests (Kind cluster required)

```bash
make test-e2e
```

### Smoke Test (in running cluster)

```bash
# Via make
make test-smoke

# Via Tilt UI
# Click [tests] → smoke-test → run
```

---

## Troubleshooting

### Tilt not installed
```bash
# Install Tilt
brew install tilt  # macOS
# Or visit https://docs.tilt.dev/tutorial.html
```

### Kind cluster won't start
```bash
# Check Docker is running
docker info

# Delete stale clusters
kind delete cluster --name korsair-dev
make tilt-up  # Try again
```

### Helm deployment fails
```bash
# Check chart syntax
helm lint ./charts/korsair/

# Dry-run to see what would be deployed
helm template korsair ./charts/korsair/ -n korsair-system
```

### Operator logs show errors
```bash
# View full logs
kubectl -n korsair-system logs -f deployment/korsair-operator --tail=100

# Check pod events
kubectl describe -n korsair-system pod -l app.kubernetes.io/component=operator
```

### Smoke test times out
```bash
# Check if CRDs are installed
kubectl get crds | grep security.blacksyrius.com

# Manually apply sample config
kubectl apply -f config/samples/security_v1alpha1_securityscanconfig.yaml

# Check if ImageScanJob was created
kubectl get imagescanjobs -A
```

---

## CI/CD & Building for Release

### Docker Hub Push (Parallel)

```bash
# Build & push operator, API, UI concurrently
make docker-build-all && make docker-push-all
```

### GitHub Actions

Workflow in `.github/workflows/docker-publish.yml`:
- **3 parallel jobs** (operator, API, UI)
- **Caching** at multiple levels (GHA, registry)
- **Multi-platform** (amd64, arm64)
- ~20 min total (vs ~50 min sequential)

**Trigger:**
```bash
gh workflow run docker-publish.yml -f version=v0.3.0
```

---

## Project Structure for Developers

```
.
├── api/v1alpha1/              # CRD definitions
├── cmd/
│   ├── main.go                # Operator entry point
│   └── web/main.go            # API server
├── internal/
│   ├── controller/            # Reconcilers & scan job logic
│   └── slack/                 # Slack webhook client
├── frontend/                  # React SPA (built into UI image)
├── charts/korsair/            # Helm chart
├── config/
│   ├── crd/                   # CRD YAML manifests
│   ├── manager/               # Manager deployment
│   ├── rbac/                  # RBAC roles & bindings
│   └── samples/               # Example CRs
├── test/                      # E2E tests
├── hack/
│   ├── setup-kind.sh          # Kind cluster initialization
│   ├── cleanup-kind.sh        # Kind cleanup
│   └── nginx-ui.conf.template # Nginx config for UI
├── Dockerfile                 # Operator binary
├── Dockerfile.api             # API server
├── Dockerfile.ui              # React + nginx
├── Tiltfile                   # Tilt dev environment
├── Makefile                   # Automation targets
└── DEVELOPMENT.md             # This file
```

---

## Next Steps

1. **Read the architecture** → `CLAUDE.md` for design decisions
2. **Understand CRDs** → `api/v1alpha1/*_types.go`
3. **Explore reconcilers** → `internal/controller/`
4. **Check the API** → `cmd/web/main.go`
5. **Run e2e tests** → `make test-e2e`

---

## Questions?

- Check logs via Tilt UI or `make tilt-logs`
- Review Makefile targets: `make help`
- See `.github/workflows/` for CI/CD patterns
- Check `CLAUDE.md` for architectural context

Happy developing! 🚀
