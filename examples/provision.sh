#!/usr/bin/env bash
# provision.sh — Bootstrap a clean two-cluster local dev environment for Korsair
#
# What this script does:
#   1. Removes any pre-existing kind-bly-hub-cluster and kind-korsair-dev clusters + contexts
#   2. Creates kind-bly-hub-cluster  (hub — Korsair Operator runs here)
#   3. Creates kind-korsair-dev      (target — workloads to be scanned)
#   4. Deploys nginx:1.25.3 demo workload to kind-korsair-dev
#
# Prerequisites: kind, kubectl
#
# Usage:
#   chmod +x examples/provision.sh
#   ./examples/provision.sh
#
set -euo pipefail

HUB_NAME="bly-hub-cluster"
DEV_NAME="korsair-dev"
HUB_CTX="kind-${HUB_NAME}"
DEV_CTX="kind-${DEV_NAME}"

info()    { echo "[INFO]  $*"; }
success() { echo "[OK]    $*"; }
warn()    { echo "[WARN]  $*"; }

require() {
  if ! command -v "$1" &>/dev/null; then
    echo "[ERROR] '$1' not found. Please install it and retry." >&2
    exit 1
  fi
}

require kind
require kubectl

# ── 1. Teardown existing clusters ────────────────────────────────────────────

info "Checking for existing clusters to remove..."

if kind get clusters 2>/dev/null | grep -q "^${HUB_NAME}$"; then
  warn "Deleting existing kind cluster '${HUB_NAME}'..."
  kind delete cluster --name "${HUB_NAME}"
fi

if kind get clusters 2>/dev/null | grep -q "^${DEV_NAME}$"; then
  warn "Deleting existing kind cluster '${DEV_NAME}'..."
  kind delete cluster --name "${DEV_NAME}"
fi

# Remove orphan contexts if they exist without a backing cluster
for ctx in "${HUB_CTX}" "${DEV_CTX}"; do
  if kubectl config get-contexts "${ctx}" &>/dev/null; then
    warn "Removing stale context '${ctx}'..."
    kubectl config delete-context "${ctx}" 2>/dev/null || true
    kubectl config delete-cluster "${ctx}" 2>/dev/null || true
    kubectl config delete-user "${ctx}" 2>/dev/null || true
  fi
done

# ── 2. Create hub cluster ────────────────────────────────────────────────────

info "Creating hub cluster '${HUB_NAME}'..."
kind create cluster --name "${HUB_NAME}" --config "$(dirname "$0")/clusters/kind-bly-hub-cluster.yaml"
success "Hub cluster ready — context: ${HUB_CTX}"

# ── 3. Create dev (target) cluster ──────────────────────────────────────────

info "Creating dev cluster '${DEV_NAME}'..."
kind create cluster --name "${DEV_NAME}" --config "$(dirname "$0")/clusters/kind-korsair-dev.yaml"
success "Dev cluster ready — context: ${DEV_CTX}"

# ── 4. Deploy demo workload to dev cluster ───────────────────────────────────

info "Deploying nginx demo workload to '${DEV_NAME}'..."
kubectl --context "${DEV_CTX}" apply -f "$(dirname "$0")/workloads/nginx-demo.yaml"
kubectl --context "${DEV_CTX}" rollout status deployment/nginx-demo -n demo --timeout=120s
success "nginx:1.25.3 is running in namespace 'demo' on ${DEV_CTX}"

# ── Done ─────────────────────────────────────────────────────────────────────

echo ""
echo "============================================================"
echo " Dev environment is ready."
echo "============================================================"
echo ""
echo "  Hub cluster (Korsair runs here):"
echo "    kubectl --context ${HUB_CTX} get nodes"
echo ""
echo "  Dev cluster (workloads to scan):"
echo "    kubectl --context ${DEV_CTX} get pods -n demo"
echo ""
echo "  Next steps:"
echo "    1. Deploy Korsair to the hub cluster:"
echo "       make dev-setup  (or: helm install korsair ./charts/korsair ...)"
echo ""
echo "    2. Register the dev cluster as a ClusterTarget:"
echo "       ./hack/add-cluster.sh korsair-dev ~/.kube/config"
echo ""
echo "    3. Apply a SecurityScanConfig:"
echo "       kubectl --context ${HUB_CTX} apply -f config/samples/security_v1alpha1_securityscanconfig.yaml"
echo ""
echo "    4. Watch scan jobs appear:"
echo "       kubectl --context ${HUB_CTX} get imagescanjobs -A -w"
echo "============================================================"
