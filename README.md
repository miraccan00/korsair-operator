# Korsair Operator

A Kubernetes-native container image vulnerability scanning operator. It automatically discovers images running in your cluster, scans them with [Trivy](https://github.com/aquasecurity/trivy) and/or [Grype](https://github.com/anchore/grype), stores results as Kubernetes resources, and sends Slack notifications when vulnerabilities exceed configurable thresholds.

## Features

- **Automatic image discovery** — scans Pods, Deployments, DaemonSets, and StatefulSets
- **Multi-scanner support** — Trivy and Grype, run independently per image
- **Multi-cluster scanning** — register remote clusters via `ClusterTarget` CRs and fan out discovery
- **Cron scheduling** — periodic scans via standard cron expressions in `SecurityScanConfig`
- **Slack notifications** — configurable cooldown and severity thresholds
- **Web dashboard** — React UI with job history, CVE tables, and cluster management
- **Least-privilege RBAC** — cluster-wide reads, namespace-scoped writes

## Architecture

See [architecture/KORSAIR_OPERATOR.md](architecture/KORSAIR_OPERATOR.md) for a full component and data-flow diagram.

```
SecurityScanConfig CR
        │
        ▼
  Operator (manager)
  ├── Discovers images from local + remote clusters
  ├── Creates ImageScanJob CRs per image × scanner
  ├── Launches batch/v1 Jobs (Trivy / Grype pods)
  └── Sends Slack notifications on threshold breach
        │
        ▼
  API Server (Go, :8090)    ◄──  UI Server (Fastify, :8080)
  /api/v1/*                       React SPA + /api/ proxy
  CVE data, job status            Dashboard, CVE tables
```

## CRDs

All CRDs are in the `security.blacksyrius.com/v1alpha1` group.

| Kind | Scope | Short name | Purpose |
|------|-------|------------|---------|
| `SecurityScanConfig` | Cluster | `ssc` | Defines what to scan, which scanners, cron schedule, and notification thresholds |
| `ImageScanJob` | Namespaced | `isj` | One scan job per image × scanner, keyed by digest |
| `ClusterTarget` | Namespaced | `ct` | Registers a remote cluster via kubeconfig Secret |
| `NotificationPolicy` | Namespaced | `np` | Slack webhook URL + per-severity threshold gating |
| `ScanPolicy` | Namespaced | `sp` | Reserved for future admission-control policy enforcement |

## Prerequisites

| Tool | Minimum version | Install |
|------|----------------|---------|
| Docker | 20.10+ | [docs.docker.com](https://docs.docker.com/get-docker/) |
| kubectl | v1.25+ | [kubernetes.io](https://kubernetes.io/docs/tasks/tools/) |
| kind | v0.20+ | [kind.sigs.k8s.io](https://kind.sigs.k8s.io/docs/user/quick-start/) |
| helm | v3.12+ | [helm.sh](https://helm.sh/docs/intro/install/) |
| Go | v1.24+ | [go.dev](https://go.dev/dl/) *(for development only)* |

## Quick Start (5 minutes)

The bootstrap script creates a local Kind cluster, builds all images, and deploys Korsair end-to-end:

```sh
git clone https://github.com/miraccan00/korsair-operator.git
cd korsair-operator
make dev-setup
```

Once deployed, apply the sample `SecurityScanConfig` to start your first scan:

```sh
kubectl apply -f config/samples/security_v1alpha1_securityscanconfig.yaml
```

Watch scan jobs appear:

```sh
kubectl get imagescanjobs -A -w
```

Run a smoke test to verify the full pipeline:

```sh
make test-smoke
```

Tear down the local cluster when done:

```sh
make dev-teardown
```

## Helm Installation (existing cluster)

```sh
helm install korsair ./charts/korsair \
  --namespace korsair-system \
  --create-namespace \
  --set operator.env.slackWebhookURL="https://hooks.slack.com/services/..."
```

Enable the web dashboard:

```sh
helm upgrade korsair ./charts/korsair \
  --namespace korsair-system \
  --set api.enabled=true \
  --set ui.enabled=true \
  --set ui.ingress.enabled=true \
  --set ui.ingress.hostname=korsair.example.com
```

See [charts/korsair/values.yaml](charts/korsair/values.yaml) for all options.

## Configuration Reference

All operator settings are passed as environment variables (via Helm values or `.env` for local dev).

| Variable | Default | Description |
|---|---|---|
| `BSO_SLACK_WEBHOOK_URL` | *(empty)* | Slack incoming webhook URL. Leave empty to disable notifications. |
| `BSO_DISCOVERY_INTERVAL` | `5m` | How often to re-discover images and requeue scans. |
| `BSO_POLL_INTERVAL` | `15s` | How often to poll running scan jobs for completion. |
| `BSO_NOTIFICATION_COOLDOWN` | `1h` | Minimum time between Slack notifications per `SecurityScanConfig`. |
| `BSO_MAX_CONCURRENT_SCANS` | `10` | Maximum simultaneously running `ImageScanJob`s. |
| `BSO_CLEANUP_INTERVAL` | `5m` | How often to garbage-collect completed/failed scan jobs. |
| `BSO_SCAN_JOB_RETENTION` | `1h` | Age after which completed/failed jobs are deleted. |

Copy [.env.example](.env.example) to `.env` for local development — the Makefile auto-loads it.

## CRD Examples

### SecurityScanConfig — scan all images in the local cluster

```yaml
apiVersion: security.blacksyrius.com/v1alpha1
kind: SecurityScanConfig
metadata:
  name: full-cluster-scan
spec:
  imageSources:
    kubernetes:
      enabled: true
      excludeNamespaces:
        - kube-system
        - kube-public
        - kube-node-lease
        - korsair-system
  scanners:
    - trivy
    - grype
  targetNamespace: korsair-system
  notificationCooldown: "1h"
  schedule: "0 2 * * *"   # daily at 02:00 UTC
```

### SecurityScanConfig — multi-cluster with discoverAllClusters

```yaml
apiVersion: security.blacksyrius.com/v1alpha1
kind: SecurityScanConfig
metadata:
  name: multi-cluster-scan
spec:
  imageSources:
    kubernetes:
      enabled: true
      excludeNamespaces: [kube-system, kube-public]
    discoverAllClusters: true   # fans out to all Connected ClusterTargets
  scanners:
    - trivy
  targetNamespace: korsair-system
```

### ClusterTarget — register a remote cluster

```yaml
apiVersion: security.blacksyrius.com/v1alpha1
kind: ClusterTarget
metadata:
  name: staging-cluster
  namespace: korsair-system
spec:
  displayName: "Staging"
  kubeconfigSecretRef:
    name: staging-kubeconfig   # Secret with key "kubeconfig"
  excludeNamespaces:
    - kube-system
```

Or use the helper script:

```sh
./hack/add-cluster.sh staging /path/to/staging-kubeconfig
```

## Scanners

### Trivy (default: enabled)

```yaml
scanners:
  trivy:
    enabled: true
    image: aquasec/trivy:0.58.1
```

### Grype (default: disabled)

```yaml
scanners:
  grype:
    enabled: true
    image: anchore/grype:v0.90.0
```

Both scanners can run simultaneously — each produces an independent `ImageScanJob`.

### Private Registry

```yaml
registryCredentials:
  enabled: true
  server: registry.example.com
  username: robot-account
  password: secret
```

Or reference an existing `kubernetes.io/dockerconfigjson` Secret:

```yaml
registryCredentials:
  enabled: true
  existingSecret: my-registry-secret
```

## Development

### Local workflow

```sh
# Run unit tests
make test

# Run the operator locally against your current kubeconfig
make run

# Run the API backend only (pair with 'cd frontend && npm run dev')
make run-api

# Lint
make lint

# Run all CI checks (fmt + vet + lint + test)
make ci
```

### Building images

```sh
# Operator
make docker-build IMG=my-registry/korsair-operator:dev

# API backend
docker build -t my-registry/korsair-operator-api:dev -f Dockerfile.api .

# UI (nginx)
docker build -t my-registry/korsair-operator-ui:dev -f Dockerfile.ui .
```

### Generating CRD manifests

```sh
make manifests   # regenerates CRDs and RBAC from Go markers
make helm-crds   # syncs generated CRDs into charts/korsair/templates/crds/
```

### Running e2e tests

```sh
make test-e2e
```

This creates an isolated Kind cluster (`korsair-operator-test-e2e`), runs the suite, and tears it down automatically.

## Scripts Reference

All helper scripts live in [hack/](hack/).

| Script | Purpose | Usage |
|---|---|---|
| [hack/setup-kind.sh](hack/setup-kind.sh) | Create Kind cluster + build + deploy end-to-end | `make dev-setup` |
| [hack/bootstrap.sh](hack/bootstrap.sh) | Redeploy to an existing cluster (idempotent) | `SKIP_BUILD=1 ./hack/bootstrap.sh` |
| [hack/rebuild-and-deploy.sh](hack/rebuild-and-deploy.sh) | Incremental image rebuild and `helm upgrade` | `./hack/rebuild-and-deploy.sh` |
| [hack/watch-and-rebuild.sh](hack/watch-and-rebuild.sh) | Auto-rebuild on Go/frontend file changes | `./hack/watch-and-rebuild.sh` |
| [hack/add-cluster.sh](hack/add-cluster.sh) | Register a remote cluster via kubeconfig | `./hack/add-cluster.sh <name> <kubeconfig>` |
| [hack/cleanup-test-cluster.sh](hack/cleanup-test-cluster.sh) | Remove all Korsair resources + Kind cluster | `make dev-teardown` |

## Uninstall

```sh
# Remove all custom resources
kubectl delete securityscanconfigs --all
kubectl delete imagescanjobs -A --all
kubectl delete clustertargets -A --all

# Uninstall Helm release and CRDs
helm uninstall korsair -n korsair-system
kubectl delete namespace korsair-system

# Or use the cleanup script (asks for confirmation)
make dev-teardown
```

## Contributing

1. Fork the repository and create a feature branch.
2. Run `make ci` to ensure all checks pass before opening a PR.
3. Follow the commit style in [CHANGELOG.md](CHANGELOG.md).
4. Open a pull request against `main`.

See [make help](#) for all available targets:

```sh
make help
```

## License

Copyright 2026 — Licensed under the [Apache License, Version 2.0](http://www.apache.org/licenses/LICENSE-2.0).
