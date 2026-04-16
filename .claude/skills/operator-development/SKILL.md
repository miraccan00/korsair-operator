# /operator-development

Guide for working on Korsair's Kubernetes operator (controller-runtime).

## Reconciler Patterns
- Return `ctrl.Result{}` on success, `ctrl.Result{RequeueAfter: d}` for polling, `err` for transient failures.
- Use `ctrl.LoggerFrom(ctx)` — never fmt.Println.
- Status updates: `r.Status().Update(ctx, obj)` — never update spec and status in the same patch.
- After editing `api/v1alpha1/` types: `make manifests && make helm-crds`.

## ImageScanJob Invariants (DO NOT BREAK)
- Job name: `scan-{digest[:8]}-{scanner}` — keyed by digest, not tag.
- Always call `resolveDigest()` before creating an ImageScanJob.
- ConfigMap CSV schema is a public contract — do not alter column order/names.
- Severity fields: `critical`, `high`, `medium`, `low` on `ImageScanJob.status`.

## Scan Lifecycle
`Pending → Running → Succeeded | Failed`
- Succeeded: parse JSON pod logs → write ConfigMap CSV → update status counts.
- Terminal: evaluate BSO_NOTIFICATION_COOLDOWN + threshold → Slack.
- GC after BSO_SCAN_JOB_RETENTION (default 1h).

## Adding a New Scanner
1. Add constant to `api/v1alpha1/types.go`.
2. Add pod spec builder in `internal/controller/scanjob_controller.go`.
3. Add JSON parser for the scanner output.
4. Add scanner image to Helm values — never hardcode version in Go.

## Key Files
- `internal/controller/securityscanconfig_controller.go` — image discovery
- `internal/controller/imagescanjob_controller.go` — job lifecycle + CSV parsing
- `internal/controller/clustertarget_controller.go` — remote cluster probing
- `internal/slack/client.go` — notification dispatch

## Commands
```
make ci            # fmt + vet + lint + tests — required before PR
make manifests     # regenerate CRDs from Go markers
make helm-crds     # copy CRDs into Helm chart
make run           # run operator against current kubeconfig
```
