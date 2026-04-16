# /api-development

Guide for working on Korsair's REST API server (`cmd/web/main.go`, port 8090).

## Stack
- Fastify (Node) or Go net/http — check current implementation in `cmd/web/main.go`.
- Serves React SPA as embedded static files.
- Proxies Kubernetes API calls through the operator's in-cluster client.

## API Conventions
- All endpoints under `/api/v1/`.
- Response format: `{"data": ..., "error": null}` or `{"data": null, "error": "message"}`.
- Auth: not yet implemented — design endpoints to accept future API_KEY header middleware.
- CRD reads via controller-runtime client (not raw kubectl) — use the manager's cache.

## Key Endpoints Pattern
- `GET /api/v1/scans` → list ImageScanJobs with status
- `GET /api/v1/scans/:name` → single ImageScanJob + CVE CSV from ConfigMap
- `GET /api/v1/configs` → list SecurityScanConfigs
- `GET /api/v1/clusters` → list ClusterTargets

## Public Contract
- Response schemas are consumed by enterprise integrations — treat as stable API.
- Do not remove or rename fields; add new fields as optional.
- CVE data comes from ConfigMap CSV — preserve field order: `id,package,version,severity,description`.

## Running Locally
```
make run-web    # start API server against current kubeconfig
```
