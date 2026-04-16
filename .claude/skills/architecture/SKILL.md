# /architecture

Answer architecture questions and plan large architectural tasks for Korsair Operator.
For complex tasks, decompose into sub-agents (see bottom of this file).

## Current System Components

```
SecurityScanConfig (cluster-scoped CR)
        │
Operator Manager (cmd/main.go)
├── SecurityScanConfigReconciler  → image discovery (local + remote clusters)
├── ImageScanJobReconciler        → batch/v1 Job lifecycle → POSTs CVE rows to API
├── ClusterTargetReconciler       → probes remote cluster kubeconfig every 5m
└── NotificationPolicyReconciler  → validates Slack webhooks

API Server (cmd/web/main.go, :8090)
├── GET  /api/v1/jobs              → lists ImageScanJobs from k8s cache
├── GET  /api/v1/jobs/:ns/:name/report → reads CVE rows from PostgreSQL
└── PUT  /api/v1/jobs/:ns/:name/report → operator writes CVE rows (internal token)

PostgreSQL (Bitnami subchart)
└── scan_reports + scan_report_rows tables → persistent CVE storage

UI Server (Fastify, :8080)
└── Serves React SPA + proxies /api/ → API Server
```

## Data Flow (current — PostgreSQL branch)

```
Running Pod
  → crane.Digest()          # digest resolution, no layer download
  → ImageScanJob CR         # keyed: scan-{digest[:8]}-{scanner}
  → batch/v1 Job            # Trivy or Grype pod
  → Pod logs (JSON)
  → operator parses JSON
  → PUT /api/v1/.../report  # operator → API server (internal token)
  → PostgreSQL              # scan_report_rows table
  → ImageScanJob.status     # critical/high/medium/low counts
  → Slack webhook           # cooldown + threshold gate
```

**ConfigMap CSV storage is REMOVED** — it was etcd-backed and did not scale beyond ~100 images.
The `report.csv` ConfigMap pattern was the v0.2–v0.5 approach; do not reintroduce it.

## Key Design Decisions

- **Digest-keyed deduplication**: same image pushed under two tags → one scan. Never change `imageToJobName()`.
- **Scanner independence**: Trivy and Grype produce separate ImageScanJob CRs, results are not merged.
- **PostgreSQL as CVE store**: replaces etcd/ConfigMap — enables historical trending, cross-cycle deduplication, SQL queries.
- **Operator → API HTTP bridge**: operator POSTs to internal API endpoint; API owns DB writes. Decouples CRD lifecycle from persistence.
- **No admission control yet**: ScanPolicy CRD is a stub for future gating.

## Scale Targets

Korsair is designed for **fleet-scale** operation: 10s to 100s of clusters connected via ClusterTarget CRs.

| Concern | Current approach | Scale limit | Future direction |
|---------|-----------------|-------------|------------------|
| Image discovery | Fan-out per ClusterTarget reconciler | ~20 clusters (serial) | Parallel discovery workers per ClusterTarget |
| Scan concurrency | `BSO_MAX_CONCURRENT_SCANS` (default 10) | Node disk / registry rate limits | Per-cluster concurrency quotas |
| CVE storage | PostgreSQL (single instance) | ~10M rows before tuning | Partitioning by `isj_namespace` + time |
| Result routing | Single API server | ~50 req/s | Horizontal API replicas + read replicas |
| Notification fan-out | One NotificationPolicy per SecurityScanConfig | Linear | Per-team routing via label selectors |

## Scalability Levers (env vars)

| Variable | Default | Tuning for scale |
|----------|---------|-----------------|
| `BSO_MAX_CONCURRENT_SCANS` | `10` | Raise per node capacity; lower if registry rate-limited |
| `BSO_DISCOVERY_INTERVAL` | `5m` | Raise to `15m`+ in large fleets to reduce API server load |
| `BSO_CLEANUP_INTERVAL` | `5m` | Raise to `30m` in high-volume environments |
| `BSO_SCAN_JOB_RETENTION` | `1h` | Reduce if many ImageScanJobs accumulate |

## Sub-Agent Decomposition for Large Tasks

When given a large architectural prompt, decompose by domain and spawn parallel sub-agents:

```
Large prompt: "Add support for 50-cluster fleet with per-team scan isolation"
  │
  ├── [Explore agent]   → read current ClusterTargetReconciler, SecurityScanConfig types
  ├── [Explore agent]   → read current PostgreSQL schema, API server endpoints
  ├── [Plan agent]      → design CRD changes for per-team isolation
  └── [Plan agent]      → design DB partitioning + API routing changes
```

### Domain → Sub-agent mapping

| Domain | Agent type | Brief with |
|--------|-----------|------------|
| Current code structure | `Explore` | "Read internal/controller/clustertarget_controller.go, SecurityScanConfig types, explain current multi-cluster fan-out" |
| DB schema / API | `Explore` | "Read cmd/web/db.go, cmd/web/main.go — explain PostgreSQL schema and API contract" |
| CRD design | `Plan` | "Design new CRD fields for X, considering public contract stability" |
| Implementation plan | `Plan` | "Plan implementation steps for X across operator + API + Helm" |

### When to spawn sub-agents vs. inline

Spawn sub-agents when the task requires **3+ file domains** read simultaneously, or when research and planning can proceed independently:
- Exploring code → `Explore` agent (fast, read-only, parallel)
- Designing changes → `Plan` agent (structured output, no file writes)
- Implementing → inline (needs sequential file edits)

Single-file edits, bug fixes, and skill updates → do inline, no sub-agent needed.

## Migration: etcd → PostgreSQL (current branch)

Branch: `feat/etcd-to-postgres-migration`

| Phase | Status | Notes |
|-------|--------|-------|
| PostgreSQL Helm subchart | ✓ Done | Bitnami, 8Gi PVC |
| DB credentials Secret | ✓ Done | `korsair-db-credentials` |
| API server DB wiring | ✓ Done | `cmd/web/db.go` |
| PUT report endpoint | ✓ Done | operator POSTs CVE rows |
| Operator ConfigMap removal | ✓ Done | ConfigMap writes removed |
| e2e tests with real DB | ⏳ Pending | Kind + PostgreSQL pod |
