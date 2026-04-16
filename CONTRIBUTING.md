# Contributing to Korsair Operator

Thank you for your interest in contributing. This guide covers everything you need to go from zero to a running local environment, run the test suite, and submit a pull request.

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Local Dev Environment](#local-dev-environment)
   - [Two-cluster setup (recommended)](#two-cluster-setup-recommended)
   - [Single-cluster setup (minimal)](#single-cluster-setup-minimal)
3. [Running Korsair Locally](#running-korsair-locally)
4. [Project Layout](#project-layout)
5. [Development Workflow](#development-workflow)
6. [Testing](#testing)
7. [Code Generation](#code-generation)
8. [Frontend Development](#frontend-development)
9. [Submitting a Pull Request](#submitting-a-pull-request)
10. [Commit Style](#commit-style)

---

## Prerequisites

Install the following tools before starting:

| Tool | Min version | Install |
|------|-------------|---------|
| Go | 1.24 | [go.dev/dl](https://go.dev/dl/) |
| Docker | 20.10 | [docs.docker.com](https://docs.docker.com/get-docker/) |
| kind | 0.20 | [kind.sigs.k8s.io](https://kind.sigs.k8s.io/docs/user/quick-start/) |
| kubectl | 1.25 | [kubernetes.io](https://kubernetes.io/docs/tasks/tools/) |
| helm | 3.12 | [helm.sh](https://helm.sh/docs/intro/install/) |
| Node.js | 20 LTS | [nodejs.org](https://nodejs.org/) *(frontend only)* |

---

## Local Dev Environment

### Two-cluster setup (recommended)

The recommended setup mirrors a real-world deployment: a **hub cluster** where Korsair runs, and a separate **target cluster** containing workloads to be scanned.

```
kind-bly-hub-cluster   ← Korsair Operator, all ImageScanJob results
kind-korsair-dev       ← nginx demo + your own test workloads
```

Bootstrap both clusters with one command:

```sh
./examples/provision.sh
```

This script:
1. Removes any pre-existing `bly-hub-cluster` / `korsair-dev` kind clusters and their kubeconfig contexts
2. Creates `kind-bly-hub-cluster` and `kind-korsair-dev` from the configs in [`examples/clusters/`](examples/clusters/)
3. Deploys `nginx:1.25.3` to the `demo` namespace on `kind-korsair-dev` — a real running image for Korsair to discover

After provisioning, verify both clusters are healthy:

```sh
kubectl --context kind-bly-hub-cluster get nodes
kubectl --context kind-korsair-dev get pods -n demo
```

#### Deploy Korsair to the hub cluster

```sh
# Build all images and deploy via Helm into kind-bly-hub-cluster
make dev-setup
```

#### Register the dev cluster as a scan target

```sh
./hack/add-cluster.sh korsair-dev ~/.kube/config
```

This creates a `ClusterTarget` CR in `korsair-system` pointing to `kind-korsair-dev`. Korsair will start discovering pods there on the next reconcile.

#### Start your first scan

```sh
kubectl --context kind-bly-hub-cluster apply -f config/samples/security_v1alpha1_securityscanconfig.yaml

# Watch ImageScanJobs appear (one per image × scanner)
kubectl --context kind-bly-hub-cluster get imagescanjobs -A -w
```

#### Tear down

```sh
kind delete cluster --name bly-hub-cluster
kind delete cluster --name korsair-dev
```

---

### Single-cluster setup (minimal)

If you only need a quick local test without multi-cluster wiring:

```sh
make dev-setup        # creates kind-korsair, deploys Korsair end-to-end
make test-smoke       # verifies an ImageScanJob is created within 60s
make dev-teardown     # removes everything
```

---

## Running Korsair Locally

Run the operator binary on your host machine (no Docker build required) against your current kubeconfig context:

```sh
# Copy and fill in env vars
cp .env.example .env

# Install CRDs, then start the operator
make install
make run
```

Run only the REST API backend (pair with `npm run dev` in `frontend/`):

```sh
make run-api
```

---

## Project Layout

```
api/v1alpha1/          CRD Go types (SecurityScanConfig, ImageScanJob, ClusterTarget, …)
archictecture/         Full ASCII architecture diagram
charts/korsair/        Helm chart
cmd/
  main.go              Operator entry point (manager + controllers)
  web/main.go          REST API + embedded React SPA
config/                Kustomize manifests, CRD YAMLs, sample CRs
examples/
  provision.sh         Two-cluster bootstrap script
  clusters/            Kind cluster configs
  workloads/           Sample Kubernetes workloads
frontend/              React 18 + TypeScript + Vite source
hack/                  Helper shell scripts
internal/
  controller/          All reconcilers + scan job cleaner
  slack/               Slack webhook client
test/                  e2e tests (Ginkgo + Kind)
```

Key controllers in [`internal/controller/`](internal/controller/):

| File | Reconciler | Responsibility |
|------|-----------|----------------|
| `securityscanconfig_controller.go` | `SecurityScanConfigReconciler` | Image discovery, `ImageScanJob` creation, Slack notifications |
| `imagescanresult_controller.go` | `ImageScanJobReconciler` | Launches Trivy/Grype pods, parses JSON output, writes CVE ConfigMap |
| `clustertarget_controller.go` | `ClusterTargetReconciler` | Probes remote cluster connectivity every 5 min |
| `notificationpolicy_controller.go` | `NotificationPolicyReconciler` | Validates Slack webhook URLs |
| `scanjob_cleaner.go` | — | Garbage-collects completed/failed jobs older than `BSO_SCAN_JOB_RETENTION` |

---

## Development Workflow

```sh
# Format code
make fmt

# Vet
make vet

# Lint (requires golangci-lint)
make lint
make lint-fix    # auto-fix where possible

# Run all CI checks at once (fmt + vet + lint + unit tests)
make ci
```

Always run `make ci` before opening a PR — the same checks run in CI.

---

## Testing

### Unit tests

```sh
make test
```

Uses [`envtest`](https://book.kubebuilder.io/reference/envtest.html) — no live cluster needed.

### Smoke test

Requires a running cluster with Korsair deployed (e.g. after `make dev-setup`):

```sh
make test-smoke
```

Applies the sample `SecurityScanConfig` and asserts that at least one `ImageScanJob` is created within 60 seconds.

### End-to-end tests

```sh
make test-e2e
```

Creates an isolated Kind cluster (`korsair-operator-test-e2e`), runs the full Ginkgo suite, then tears the cluster down automatically.

---

## Code Generation

After modifying CRD types in `api/v1alpha1/` or changing controller-gen markers:

```sh
# Regenerate DeepCopy methods
make generate

# Regenerate CRD manifests + RBAC
make manifests

# Sync generated CRDs into the Helm chart
make helm-crds
```

Always commit generated files together with the type changes that produced them.

---

## Frontend Development

The React UI lives in [`frontend/`](frontend/) and is served by the API server when built with the `webui` build tag.

```sh
# Hot-reload development (proxies API calls to :8090)
cd frontend
npm ci
npm run dev       # http://localhost:5173

# In a separate terminal — run only the API backend
make run-api      # http://localhost:8090

# Production build (output → cmd/web/static/dist/)
make build-frontend
```

The production build is embedded into the Go binary at compile time via `//go:embed`.

---

## Submitting a Pull Request

1. **Fork** the repository and create a feature branch off `main`.
2. Make your changes. Run `make ci` to ensure all checks pass.
3. Add or update tests to cover your change.
4. If you changed CRD types, run `make generate && make manifests && make helm-crds` and commit the result.
5. Open a pull request against `main` with a clear description of what and why.

**Do not** break the `ImageScanJob` CSV report schema — downstream integrations depend on it as a stable contract. If a schema change is unavoidable, call it out explicitly in the PR description.

---

## Commit Style

Follow the format used in [CHANGELOG.md](CHANGELOG.md):

```
<type>: <short summary in imperative mood>

[optional body]
```

Common types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`.

Examples:
```
feat: add Grype scanner support to ImageScanJobReconciler
fix: prevent duplicate ImageScanJobs when tag resolves to existing digest
docs: add ClusterTarget registration example to README
```

Keep the subject line under 72 characters. Reference GitHub issues where relevant (`Closes #42`).
