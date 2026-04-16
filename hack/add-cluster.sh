#!/usr/bin/env bash
# hack/add-cluster.sh — Register a remote cluster kubeconfig with Korsair
#
# Usage:
#   ./hack/add-cluster.sh <display-name> <path/to/kubeconfig.yaml>
#
# The kubeconfig is uploaded to the Korsair web API, which creates:
#   - A Secret containing the kubeconfig in korsair-system
#   - A ClusterTarget CR pointing to the remote cluster
#
# After adding, the operator discovers images from the remote cluster
# and creates ImageScanJobs for them alongside local cluster images.

set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BOLD='\033[1m'; NC='\033[0m'
die()  { echo -e "${RED}✗ ERROR:${NC} $*" >&2; exit 1; }
ok()   { echo -e "${GREEN}✓${NC} $*"; }
info() { echo -e "${BOLD}▶${NC} $*"; }

# ─── Config ───────────────────────────────────────────────────────────────────
DASHBOARD_HOST="${DASHBOARD_HOST:-korsair.local}"
DASHBOARD_PORT="${DASHBOARD_PORT:-8080}"
API_BASE="http://${DASHBOARD_HOST}:${DASHBOARD_PORT}/api/v1"

# ─── Args ─────────────────────────────────────────────────────────────────────
DISPLAY_NAME="${1:-}"
KUBECONFIG_PATH="${2:-}"

if [[ -z "${DISPLAY_NAME}" || -z "${KUBECONFIG_PATH}" ]]; then
  echo "Usage: $0 <display-name> <path/to/kubeconfig.yaml>"
  echo ""
  echo "  display-name      Human-readable label shown in the dashboard"
  echo "  kubeconfig.yaml   Path to the target cluster's kubeconfig file"
  echo ""
  echo "Environment:"
  echo "  DASHBOARD_HOST    Dashboard hostname (default: korsair.local)"
  echo "  DASHBOARD_PORT    Dashboard port     (default: 8080)"
  exit 1
fi

[[ -f "${KUBECONFIG_PATH}" ]] || die "Kubeconfig file not found: ${KUBECONFIG_PATH}"

# ─── Upload ───────────────────────────────────────────────────────────────────
info "Registering cluster '${DISPLAY_NAME}' from ${KUBECONFIG_PATH} ..."
info "API endpoint: ${API_BASE}/clusters"

HTTP_STATUS=$(curl -s -o /tmp/korsair-add-cluster-response.json -w "%{http_code}" \
  -X POST "${API_BASE}/clusters" \
  -F "displayName=${DISPLAY_NAME}" \
  -F "kubeconfig=@${KUBECONFIG_PATH}")

RESPONSE=$(cat /tmp/korsair-add-cluster-response.json 2>/dev/null || echo "{}")

if [[ "${HTTP_STATUS}" == "200" || "${HTTP_STATUS}" == "201" ]]; then
  ok "Cluster '${DISPLAY_NAME}' registered successfully (HTTP ${HTTP_STATUS})"
  echo ""
  echo "  Verify: kubectl get clustertargets -n korsair-system"
  echo "  Logs:   kubectl logs -n korsair-system -l app.kubernetes.io/component=operator -f"
else
  echo -e "${RED}✗ Registration failed (HTTP ${HTTP_STATUS})${NC}"
  echo "  Response: ${RESPONSE}"
  echo ""
  echo "Troubleshooting:"
  echo "  1. Is the dashboard running?  curl ${API_BASE}/summary"
  echo "  2. Is ${DASHBOARD_HOST}:${DASHBOARD_PORT} reachable?"
  echo "     Alternative: kubectl port-forward -n korsair-system svc/korsair-web 8090:8090"
  echo "     Then: DASHBOARD_HOST=localhost DASHBOARD_PORT=8090 $0 ${DISPLAY_NAME} ${KUBECONFIG_PATH}"
  exit 1
fi
