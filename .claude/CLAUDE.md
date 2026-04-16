# Korsair Operator — Claude Context

## Mission
Open-source Kubernetes-native supply-chain security layer. Continuously re-scans every running container image in a cluster (or fleet), surfacing new CVEs against digests that passed CI gates at deploy time.

- API group: `security.blacksyrius.com/v1alpha1`
- Module: `github.com/miraccan00/korsair-operator`
- No proprietary references, internal URLs, or closed-source dependencies.
- Public contracts: CRD schemas, ImageScanJob CSV report, REST API — no breaking changes.

## Roadmap
Enterprise integration via `API_KEY` — SIEMs/GRC tools invoke Trivy/Grype through the Korsair API without their own scanning infra.

## Architecture

```
SecurityScanConfig (cluster-scoped CR)
        │
Operator Manager (cmd/main.go)
├── SecurityScanConfigReconciler  → discovers images → creates ImageScanJobs
├── ImageScanJobReconciler        → batch/v1 Job (Trivy/Grype pod) → parses logs → ConfigMap CSV
├── ClusterTargetReconciler       → probes remote clusters every 5m
└── NotificationPolicyReconciler  → validates Slack webhooks

API Server (cmd/web/main.go :8090) → React SPA (frontend/)
```

### CRDs
| CRD | Scope | Purpose |
|-----|-------|---------|
| `SecurityScanConfig` | Cluster | Scanners, cron schedule, notification thresholds |
| `ImageScanJob` | Namespaced | One job per image×scanner, keyed by digest |
| `ClusterTarget` | Namespaced | Remote cluster via kubeconfig Secret |
| `NotificationPolicy` | Namespaced | Slack webhook + severity threshold |
| `ScanPolicy` | Namespaced | Stub — future admission-control |

### Scan Flow
1. Reconciler lists running pods (local + remote via ClusterTarget)
2. Resolves image digest via `crane` (google/go-containerregistry)
3. Creates `ImageScanJob` named `scan-{digest[:8]}-{scanner}` — deduplicated by digest, not tag
4. Launches `batch/v1 Job` with scanner pod
5. Reads pod logs → parses JSON → writes CVE CSV to ConfigMap
6. Updates `ImageScanJob.status` with critical/high/medium/low counts
7. Terminal state → evaluates cooldown + threshold → Slack webhook

### Scanners
- **Trivy** `aquasec/trivy:0.58.1` — `image --format json --exit-code 0 --timeout 10m <image>`
- **Grype** `anchore/grype:v0.90.0` — `<image> -o json --quiet`

## Tech Stack
| Layer | Technology |
|-------|-----------|
| Operator | Go 1.25, controller-runtime v0.23.1, client-go v0.35.0 |
| Registry | google/go-containerregistry v0.21.0 (crane) |
| Scheduling | robfig/cron/v3 |
| Frontend | React 18, TypeScript, Vite, Tailwind CSS |
| Notifications | Slack Incoming Webhooks (`internal/slack/client.go`) |
| Packaging | Helm (`charts/korsair/`), Kustomize (`config/`) |
| Testing | Ginkgo v2 + Gomega, envtest |

## Env Vars
| Variable | Default | Description |
|----------|---------|-------------|
| `BSO_DISCOVERY_INTERVAL` | `5m` | Image re-discovery frequency |
| `BSO_POLL_INTERVAL` | `15s` | Scan job completion polling |
| `BSO_NOTIFICATION_COOLDOWN` | `1h` | Min time between Slack alerts per config |
| `BSO_SLACK_WEBHOOK_URL` | — | Empty = notifications disabled |
| `BSO_MAX_CONCURRENT_SCANS` | `10` | Max concurrent ImageScanJobs |
| `BSO_CLEANUP_INTERVAL` | `5m` | GC interval for completed/failed jobs |
| `BSO_SCAN_JOB_RETENTION` | `1h` | Age threshold before job deletion |

## Directory Layout
```
api/v1alpha1/       CRD Go types + DeepCopy
charts/korsair/     Helm chart
cmd/main.go         Operator entry point
cmd/web/main.go     REST API + React SPA embed (:8090)
config/             Kustomize manifests + CRD YAMLs
frontend/           React 18 + Vite
hack/               Dev helper scripts
internal/controller/ Reconcilers + scan job cleaner
internal/slack/     Slack webhook client
test/               e2e tests (Ginkgo + Kind)
```

## Dev Conventions
- `make ci` — fmt, vet, lint, unit tests — run before every PR
- `make dev-setup` — full local Kind cluster (≈5 min)
- `make run` — operator against current kubeconfig
- `make manifests && make helm-crds` — regenerate CRD/RBAC from Go markers

## Hard Rules
- Never break `ImageScanJob` CSV schema — downstream consumers depend on it
- Never skip `resolveDigest()` — tag scanning breaks digest deduplication
- Never change `imageToJobName()` keying without a migration plan
- Scanner versions belong in Helm values / CRD spec, not business logic
- No env-var feature flags — change code directly
