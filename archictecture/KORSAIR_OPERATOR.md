╔══════════════════════════════════════════════════════════════════════════════════════════╗                                       
  ║                         KORSAIR OPERATOR — v0.5.0 Architecture                          ║                                        
  ║                           security.blacksyrius.com/v1alpha1                             ║                                        
  ╚══════════════════════════════════════════════════════════════════════════════════════════╝                                       
                                                                                                                                     
  ┌─────────────────────────────────────────────────────────────────────────────────────────┐                                        
  │                                   USER / OPERATOR                                       │
  │                                                                                         │                                        
  │   kubectl apply -f securityscanconfig.yaml          http://localhost:5173 (Web UI)      │                                        
  │   kubectl apply -f clustertarget.yaml               http://localhost:8090 (API)         │
  └──────────────────┬──────────────────────────────────────────┬──────────────────────────┘
                     │ CRs                                       │ REST
                     ▼                                           ▼
  ╔══════════════════════════════╗              ╔═══════════════════════════════════════════╗
  ║   KUBERNETES API SERVER      ║              ║          cmd/web/main.go  :8090           ║
  ║                              ║              ║                                           ║
  ║  CRDs (cluster-scoped):      ║              ║  GET  /api/v1/summary                    ║
  ║  • SecurityScanConfig        ║◄─────────────║  GET  /api/v1/configs                    ║
  ║                              ║              ║  GET  /api/v1/jobs[?namespace=]          ║
  ║  CRDs (namespaced):          ║              ║  GET  /api/v1/jobs/{ns}/{name}/report    ║
  ║  • ImageScanJob              ║◄─────────────║  GET  /api/v1/clusters                   ║
  ║  • ClusterTarget             ║              ║  POST /api/v1/clusters                   ║
  ║  • NotificationPolicy        ║              ║  DEL  /api/v1/clusters/{name}            ║
  ║  • ScanPolicy (stub)         ║              ║                                           ║
  ║                              ║              ║  Serve React SPA (-tags webui)           ║
  ║  Core Resources:             ║              ╚═══════════════════════════════════════════╝
  ║  • Pod (read)                ║                             │
  ║  • batch/v1 Job              ║                             │ serves
  ║  • ConfigMap (report)        ║                             ▼
  ║  • Secret (kubeconfig)       ║              ╔═══════════════════════════════════════════╗
  ║  • Namespace                 ║              ║      frontend/src/  (React 18 + Vite)     ║
  ╚══════════════════════════════╝              ║                                           ║
               ▲  ▲  ▲  ▲                      ║  Pages:                                   ║
               │  │  │  │                      ║  ├── Dashboard    (summary + recent jobs) ║
               │  │  │  │                      ║  ├── JobsPage     (filter: phase/scanner/ ║
               │  │  │  │     watches/manages  ║  │                  namespace/cluster)    ║
               │  │  │  │                      ║  ├── JobDetailPage(CVE table, severity    ║
  ╔════════════╪══╪══╪══╪════════════════════╗ ║  │                  chart, NVD links)    ║
  ║            │  │  │  │   cmd/main.go       ║ ║  ├── ConfigsPage  (SecurityScanConfigs) ║
  ║   ┌────────┴──┴──┘  │   OPERATOR MANAGER  ║ ║  └── ClustersPage (ClusterTarget mgmt) ║
  ║   │                 │                     ║ ║                                           ║
  ║   │   ┌─────────────┘   env vars:         ║ ║  Components:                             ║
  ║   │   │                 BSO_DISCOVERY_    ║ ║  JobsTable, PhaseBadge, SeverityBadge   ║
  ║   │   │                 INTERVAL (5m)     ║ ║  SummaryCard, Navbar, ErrorMessage       ║
  ║   │   │                 BSO_POLL_INTERVAL ║ ║                                           ║
  ║   │   │                 (15s)             ║ ║  auto-refresh: 30s (useAutoRefresh hook) ║
  ║   │   │                 BSO_NOTIFICATION_ ║ ╚═══════════════════════════════════════════╝
  ║   │   │                 COOLDOWN (1h)     ║
  ║   │   │                 BSO_SLACK_WEBHOOK ║
  ║   │   │                                  ║
  ║   ▼   ▼                                  ║
  ║ ┌─────────────────────────────────────┐  ║
  ║ │  SecurityScanConfigReconciler       │  ║
  ║ │  internal/controller/              │  ║
  ║ │  securityscanconfig_controller.go  │  ║
  ║ │                                    │  ║
  ║ │  1. List local pods (excludeNS)    │  ║
  ║ │     → map[image]sourceNamespace    │  ║
  ║ │                                    │  ║
  ║ │  2. discoverAllClusters=true?      │  ║
  ║ │     List Connected ClusterTargets  │  ║    ┌────────────────────────────┐
  ║ │     For each ClusterTarget:        │  ║    │   REMOTE CLUSTER           │
  ║ │     • Load kubeconfig Secret       │  ║    │                            │
  ║ │     • Build remote client          │──╫────►  List pods                │
  ║ │     • discoverImagesFromClient()   │  ║    │  (excludeNamespaces)       │
  ║ │     • Merge images + label         │  ║    │                            │
  ║ │       cluster-target=<name>        │  ║    │  return map[image]ns       │
  ║ │                                    │  ║    └────────────────────────────┘
  ║ │  3. For each image × scanner:      │  ║
  ║ │     • resolveDigest() [crane,30s]  │  ║
  ║ │       → sha256:xxxx (or fallback)  │  ║
  ║ │     • imageToJobName()             │  ║
  ║ │       scan-{digest[:8]}-{scanner}  │  ║
  ║ │     • ensureImageScanJob()         │  ║
  ║ │       create / skip / patch(migr.) │  ║
  ║ │                                    │  ║
  ║ │  4. updateStatus()                 │  ║
  ║ │     Notification gate:             │  ║    ┌──────────────────────────┐
  ║ │     • all terminal?                │  ║    │  internal/slack/client.go│
  ║ │     • completed > 0?              │  ║    │                          │
  ║ │     • cooldown elapsed?            │──╫────► SendScanReport()        │
  ║ │     • new images?                  │  ║    │                          │
  ║ │     • thresholds exceeded?         │  ║    │  POST webhookURL         │
  ║ │     • webhook set?                 │  ║    │  → Slack Incoming Webhook│
  ║ │     → Slack notification           │  ║    └──────────────────────────┘
  ║ │                                    │  ║
  ║ │  5. Requeue                        │  ║
  ║ │     cron schedule OR interval      │  ║
  ║ └──────────────┬─────────────────────┘  ║
  ║                │ creates                ║
  ║                ▼                        ║
  ║ ┌──────────────────────────────────┐    ║
  ║ │        ImageScanJob              │    ║
  ║ │  spec:                           │    ║
  ║ │    image: nginx:1.25             │    ║
  ║ │    scanner: trivy                │    ║
  ║ │    imageDigest: sha256:...       │    ║
  ║ │    sourceNamespace: default      │    ║
  ║ │    configRef: cluster-scan       │    ║
  ║ │    source: kubernetes            │    ║
  ║ │  labels:                         │    ║
  ║ │    cluster-target: remote-prod   │    ║
  ║ │  status:                         │    ║
  ║ │    phase: Pending→Running→       │    ║
  ║ │           Completed/Failed       │    ║
  ║ │    criticalCount: 2              │    ║
  ║ │    highCount: 5                  │    ║
  ║ └──────────────┬───────────────────┘    ║
  ║                │ watches                ║
  ║                ▼                        ║
  ║ ┌─────────────────────────────────────┐ ║
  ║ │  ImageScanJobReconciler             │ ║
  ║ │  imagescanresult_controller.go      │ ║
  ║ │                                     │ ║
  ║ │  1. Find/create batch/v1 Job        │ ║
  ║ │     scan-{name}-trivy               │ ║    ┌─────────────────────────┐
  ║ │     aquasec/trivy:0.58.1            │─╫────► TRIVY POD               │
  ║ │     anchore/grype:v0.90.0           │ ║    │ image --format json     │
  ║ │                                     │ ║    │ --exit-code 0           │
  ║ │  2. Poll Job status (15s)           │ ║    │ --timeout 10m <image>   │
  ║ │                                     │ ║    └──────────┬──────────────┘
  ║ │  3. On success:                     │ ║               │ JSON output
  ║ │     • readPodLogs() [clientset]     │◄╫───────────────┘
  ║ │     • parseTrivyJSON()              │ ║    ┌─────────────────────────┐
  ║ │       parseGrypeJSON()              │ ║    │ GRYPE POD               │
  ║ │     • count severities              │─╫────► <image> -o json --quiet │
  ║ │     • writeCSVConfigMap()           │ ║    └─────────────────────────┘
  ║ │       → ConfigMap {name}-report     │ ║
  ║ │         report.csv:                 │ ║
  ║ │         Image,Target,Library,       │ ║
  ║ │         VulnID,Severity,...         │ ║
  ║ │     • markCompleted(counts)         │ ║
  ║ │                                     │ ║
  ║ │  4. On failure: markFailed()        │ ║
  ║ └─────────────────────────────────────┘ ║
  ║                                         ║
  ║ ┌─────────────────────────────────────┐ ║
  ║ │  ClusterTargetReconciler            │ ║
  ║ │  clustertarget_controller.go        │ ║
  ║ │                                     │ ║    ┌──────────────────────────┐
  ║ │  Every 5 minutes:                   │ ║    │   REMOTE CLUSTER         │
  ║ │  • Load kubeconfig Secret           │─╫────► probeCluster()           │
  ║ │  • probeCluster() [15s timeout]     │ ║    │ List nodes               │
  ║ │  • status.phase = Connected/Error   │ ║    └──────────────────────────┘
  ║ │  • status.nodeCount                 │ ║
  ║ └─────────────────────────────────────┘ ║
  ║                                         ║
  ║ ┌─────────────────────────────────────┐ ║
  ║ │  NotificationPolicyReconciler       │ ║
  ║ │  notificationpolicy_controller.go   │ ║
  ║ │                                     │ ║
  ║ │  • Validate webhookURL (https://)   │ ║
  ║ │  • Set Ready condition              │ ║
  ║ └─────────────────────────────────────┘ ║
  ╚═════════════════════════════════════════╝

  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
    END-TO-END SCAN FLOW
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

    SecurityScanConfig ──► discoverImages()
          │                (local + remote clusters)
          │                       │
          │                map[image → sourceNS]
          │                       │
          ▼                       ▼
    resolveDigest()        imageToJobName()
    crane HEAD request  →  scan-{digest[:8]}-trivy
    sha256:abcd1234...     scan-{digest[:8]}-grype
          │
          ▼
    ensureImageScanJob()    ──────────────────────────────────┐
    (create / skip /                                          │
     patch sourceNamespace)                                   │
          │                                                   │
          ▼                                                   │
    ImageScanJob created                                      │ watches
    (Pending)                                                 │
          │                                                   │
          ▼                                    ───────────────┘
    ImageScanJobReconciler                     │
    createTrivyJob() / createGrypeJob()        │ ISJ completion event
    batch/v1 Job → Pod runs scanner            │ → triggers SSC reconcile
          │                                   │
          ▼                                   │
    Parse JSON logs                           │
    → CVE counts + vulnRecords               │
    → ConfigMap {name}-report.csv            │
    → markCompleted()                        │
          │                                   │
          └────────────────────────────────── ► updateStatus()
                                                cooldown + threshold gate
                                                      │
                                                      ▼
                                               Slack Webhook
                                               (batch notification)

  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
    DEDUPLICATION (v0.4+)
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

    nginx:1.25  ─┐
                 ├─► crane.Digest() ─► sha256:abc12345...
    nginx:latest ─┘                         │
                                            ▼
                                     scan-abc12345-trivy  ← same job!
                                     (one scan per digest)

  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
    TECH STACK
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

    Operator   Go 1.25 · controller-runtime v0.23.1 · client-go v0.35.0
    Scanners   aquasec/trivy:0.58.1 · anchore/grype:v0.90.0
    Registry   google/go-containerregistry v0.21.0 (crane)
    Schedule   robfig/cron/v3
    Frontend   React 18 · TypeScript · Vite · Tailwind CSS · React Router
    Deploy     Kustomize · Helm (charts/bso/) · Distroless container