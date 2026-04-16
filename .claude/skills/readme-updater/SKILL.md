# /readme-updater

Update README.md and project documentation for Korsair Operator.

## README Sections to Keep Current
1. **What is Korsair** — one-paragraph mission (no internal references)
2. **Quick Start** — `make dev-setup` then apply sample CRs from `config/samples/`
3. **CRDs** — table with all 5 CRDs, scope, and purpose
4. **Scanners** — Trivy + Grype with current image versions from Helm values
5. **Configuration** — env var table (match `CLAUDE.md` env vars section)
6. **Contributing** — `make ci` before PR, markers for CRD generation

## Rules
- No proprietary/internal references.
- Scanner versions come from `charts/korsair/values.yaml` — do not hardcode in prose.
- Sample CR snippets must match current `config/samples/` files.
- Helm install command must use the chart in `charts/korsair/`.

## When Updating
1. Read `charts/korsair/values.yaml` for current image tags.
2. Read `config/samples/` for current CR examples.
3. Read `api/v1alpha1/types.go` for current CRD fields.
4. Update README.md to match — do not introduce new information not in code.
