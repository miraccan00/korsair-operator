# /local-devcontainer-tilftfile-updater

Update `.devcontainer/` and `Tiltfile` for local development environment.

## Devcontainer Setup
- Base: Go + Node devcontainer with Kind and kubectl pre-installed.
- PostgreSQL sidecar for future etcd-to-postgres migration (current branch: `feat/etcd-to-postgres-migration`).
- Env vars from `.env` (gitignored) — template in `.env.example`.

## Tiltfile Conventions
- `k8s_yaml()` for CRD manifests from `config/crd/`.
- `docker_build()` for operator image — use `live_update` for fast iteration.
- `k8s_resource()` to set port-forwards: operator metrics (:8080), API server (:8090).
- Helm chart deployment via `helm_resource()` for `charts/korsair/`.

## When Updating
1. Check `hack/dev-setup.sh` — Tiltfile should mirror that setup.
2. After adding a new CRD: add it to `k8s_yaml()` in Tiltfile and devcontainer init script.
3. After adding a new env var: add it to `.env.example` with a safe default.
4. Test with `make dev-setup` to verify the full flow.

## Key Files
- `.devcontainer/devcontainer.json`
- `.devcontainer/Dockerfile` (if custom)
- `Tiltfile`
- `.env.example`
- `hack/dev-setup.sh`
