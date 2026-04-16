# /test-architecture

Review and enforce the AAA (Arrange-Act-Assert) pattern across Korsair test files.
This skill is for **code review and architectural compliance** — test execution belongs to `/pre-commit`.

## AAA Structure — Required in Every Test

Each `It(...)` block must have three clearly separated phases:

```go
It("should write severity counts to status on job success", func() {
    // ── Arrange ──────────────────────────────────────────────────────
    // All preconditions: objects, fakes, context. No mutations here.
    job := &securityv1alpha1.ImageScanJob{
        ObjectMeta: metav1.ObjectMeta{Name: "scan-abc12345-trivy", Namespace: "korsair-system"},
        Spec:       securityv1alpha1.ImageScanJobSpec{Image: "nginx:1.21", Scanner: "trivy"},
    }
    Expect(k8sClient.Create(ctx, job)).To(Succeed())

    // ── Act ───────────────────────────────────────────────────────────
    // One logical state mutation per test. Async → use Eventually, not time.Sleep.
    batchJob.Status.Succeeded = 1
    Expect(k8sClient.Status().Update(ctx, batchJob)).To(Succeed())

    // ── Assert ────────────────────────────────────────────────────────
    // Observable outcomes only: status fields, created objects, ConfigMap data.
    // Never assert internal state, log lines, or method call counts.
    Eventually(func(g Gomega) {
        g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)).To(Succeed())
        g.Expect(job.Status.Phase).To(Equal(securityv1alpha1.PhaseSucceeded))
        g.Expect(job.Status.CriticalCount).To(BeNumerically(">", 0))
    }, timeout, interval).Should(Succeed())
})
```

## AAA Rules

| Rule | Correct | Wrong |
|------|---------|-------|
| One Act per test | Single state mutation | Multiple mutations in one It |
| Async assertions | `Eventually(func(g Gomega){...})` | `time.Sleep` + `Expect` |
| Isolation | `AfterEach` cleans up objects | shared mutable state between Its |
| Assert surface | Status fields, created CRs, ConfigMap CSV | Internal struct fields, log output |
| Phase comments | `// ── Arrange ──` on its own line | Inline or missing |

## Table-Driven Tests (DescribeTable)

```go
DescribeTable("imageToJobName deduplication",
    func(image, scanner, digest, expected string) {
        // Arrange: inputs come from the Entry — no setup needed
        // Act
        result := imageToJobName(image, scanner, digest)
        // Assert
        Expect(result).To(Equal(expected))
    },
    Entry("digest prefix used",       "nginx:1.25",   "trivy", "sha256:09fb0c62", "scan-09fb0c62-trivy"),
    Entry("same digest → same name",  "nginx:latest", "trivy", "sha256:09fb0c62", "scan-09fb0c62-trivy"),
    Entry("different scanner",        "nginx:1.25",   "grype", "sha256:09fb0c62", "scan-09fb0c62-grype"),
)
```

## Test Layers

| Layer | Location | Tool | AAA scope |
|-------|----------|------|-----------|
| Unit | `internal/controller/*_test.go` | Ginkgo v2 + Gomega + envtest | One reconciler behaviour per It |
| API | `cmd/web/api_test.go` | stdlib `testing` + sqlmock | One HTTP handler per test func |
| e2e | `test/` | Ginkgo + Kind | Full pipeline, one scenario per It |

## Review Checklist

When reviewing test code, verify:
- [ ] Every `It` has all three AAA phases with `// ── Phase ──` comment separators.
- [ ] No `time.Sleep` — all async assertions use `Eventually`.
- [ ] `Eventually` uses the `func(g Gomega)` form (not closure captures) for atomicity.
- [ ] No duplicate literal strings ≥3 times — extract to `const` block above the `Describe`.
- [ ] `AfterEach` deletes all objects created in `It` or `BeforeEach`.
- [ ] Each `It` tests exactly one behaviour — if the description needs "and", split it.
- [ ] Assert only observable Kubernetes state — not return values of private functions.

## Invariants to Always Test

These behaviours must have dedicated test cases:

| Invariant | What to assert |
|-----------|---------------|
| Digest deduplication | Same digest+scanner → only one ImageScanJob created |
| Reconciler idempotency | Running reconcile twice → no duplicate resources |
| CSV schema | ConfigMap `report.csv` has exact header order |
| Notification cooldown | Second notification within cooldown window → not sent |

## What This Skill Does NOT Cover

- Running tests → use `/pre-commit`
- Setting up envtest / Kind → see `make dev-setup`
- Writing reconciler logic → use `/operator-development`
