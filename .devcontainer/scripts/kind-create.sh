#!/usr/bin/env bash
# .devcontainer/scripts/kind-create.sh
#
# Creates (or recreates) a kind cluster inside the devcontainer and wires the
# devcontainer to the kind Docker network so it can reach the API server via its
# internal IP — no host.docker.internal port-forwarding chain needed.
#
# Usage:
#   bash .devcontainer/scripts/kind-create.sh              # cluster name: korsair-dev
#   bash .devcontainer/scripts/kind-create.sh my-cluster   # custom name

set -euo pipefail

CLUSTER_NAME="${1:-korsair-dev}"
KUBECONFIG_FILE="${HOME}/.kube/config"

# kind-config.yaml lives in .devcontainer/ because it sets apiServerAddress:
# "0.0.0.0" so the control-plane listens on all interfaces — necessary for the
# devcontainer to reach it via the internal Docker IP. The project's
# examples/clusters/kind-korsair-dev.yaml binds to 127.0.0.1 (host-only).
KIND_CONFIG="$(cd "$(dirname "$0")/.." && pwd)/kind-config.yaml"

# ── 1. Create cluster if not already present ─────────────────────────────────
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
  echo "==> Cluster '${CLUSTER_NAME}' already exists — skipping create"
else
  echo "==> Creating kind cluster '${CLUSTER_NAME}'"
  echo "    Config: ${KIND_CONFIG}"
  echo ""
  kind create cluster \
    --name "${CLUSTER_NAME}" \
    --config "${KIND_CONFIG}" \
    --wait 90s
fi

# ── 3. Connect devcontainer to the kind Docker network ────────────────────────
# hostname inside a devcontainer resolves to its Docker container ID.
echo ""
echo "==> Connecting devcontainer to kind network..."
CONTAINER_ID=$(hostname)
docker network connect kind "${CONTAINER_ID}" 2>/dev/null \
  && echo "    Connected (container=${CONTAINER_ID})" \
  || echo "    Already connected — skipping"

# ── 4. Patch kubeconfig to use the control-plane's internal IP ───────────────
echo ""
KIND_IP=$(docker inspect "${CLUSTER_NAME}-control-plane" \
  --format '{{.NetworkSettings.Networks.kind.IPAddress}}')
echo "==> API server internal IP: ${KIND_IP}"

echo ""
echo "==> Patching kubeconfig..."
kubectl config set-cluster "kind-${CLUSTER_NAME}" \
  --server="https://${KIND_IP}:6443" \
  --insecure-skip-tls-verify=true \
  --kubeconfig="${KUBECONFIG_FILE}"

# certificate-authority-data conflicts with insecure-skip-tls-verify — remove it.
kubectl config unset "clusters.kind-${CLUSTER_NAME}.certificate-authority-data" \
  --kubeconfig="${KUBECONFIG_FILE}" 2>/dev/null || true

# ── 5. Set current context + verify ──────────────────────────────────────────
echo ""
echo "==> Setting current context to kind-${CLUSTER_NAME}..."
kubectl config use-context "kind-${CLUSTER_NAME}" --kubeconfig="${KUBECONFIG_FILE}"

echo ""
echo "==> Verifying cluster access..."
kubectl cluster-info --context "kind-${CLUSTER_NAME}" --kubeconfig="${KUBECONFIG_FILE}"
echo ""
kubectl get nodes -o wide --context "kind-${CLUSTER_NAME}" --kubeconfig="${KUBECONFIG_FILE}"
echo ""
echo "==> Cluster '${CLUSTER_NAME}' is ready!"
echo "    API server: https://${KIND_IP}:6443"
echo ""
echo "    Next steps:"
echo "      tilt up -f Tiltfile-local    # full-stack dev"
echo "      kubectl get pods -A          # check running pods"
