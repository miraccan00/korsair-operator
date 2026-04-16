#!/usr/bin/env bash
# hack/cleanup-test-cluster.sh — Remove all Korsair resources from a remote cluster.
#
# Usage:
#   ./hack/cleanup-test-cluster.sh                         # uses current kubectl context
#   KUBE_CONTEXT=test ./hack/cleanup-test-cluster.sh       # specific context
#
# This script removes:
#   - SecurityScanConfig CRs (cluster-scoped)
#   - korsair-system namespace (and all namespaced resources inside it)
#   - All Korsair CRDs
#
# Run this when migrating from "operator in cluster" to "operator in Kind + ClusterTarget".

set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BOLD='\033[1m'; NC='\033[0m'
die()  { echo -e "${RED}✗ ERROR:${NC} $*" >&2; exit 1; }
ok()   { echo -e "${GREEN}✓${NC} $*"; }
info() { echo -e "${BOLD}▶${NC} $*"; }
warn() { echo -e "${YELLOW}⚠${NC} $*"; }

KUBE_CONTEXT="${KUBE_CONTEXT:-$(kubectl config current-context)}"
KUBECTL="kubectl --context ${KUBE_CONTEXT}"

echo ""
echo -e "${BOLD}${RED}Korsair Cluster Cleanup${NC}"
echo -e "Target context: ${BOLD}${KUBE_CONTEXT}${NC}"
echo ""
warn "This will permanently remove all Korsair CRDs and the korsair-system namespace."
read -r -p "Continue? [y/N] " answer
[[ "${answer,,}" == "y" ]] || { echo "Aborted."; exit 0; }
echo ""

# ── SecurityScanConfig (cluster-scoped) ───────────────────────────────────────
info "Removing SecurityScanConfig CRs..."
if $KUBECTL get crd securityscanconfigs.security.blacksyrius.com &>/dev/null; then
  $KUBECTL delete securityscanconfigs --all --ignore-not-found 2>&1 && ok "SecurityScanConfigs deleted" || warn "Could not delete SecurityScanConfigs"
else
  warn "SecurityScanConfig CRD not found — skipping"
fi

# ── korsair-system namespace (contains ISJs, Trivy jobs, secrets) ─────────────
info "Removing korsair-system namespace..."
if $KUBECTL get namespace korsair-system &>/dev/null; then
  $KUBECTL delete namespace korsair-system --wait=true 2>&1 && ok "korsair-system namespace deleted" || warn "Namespace deletion may still be in progress"
else
  warn "korsair-system namespace not found — already clean"
fi

# ── CRDs ──────────────────────────────────────────────────────────────────────
info "Removing Korsair CRDs..."
CRDS=(
  "clustertargets.security.blacksyrius.com"
  "imagescanjobs.security.blacksyrius.com"
  "notificationpolicies.security.blacksyrius.com"
  "scanpolicies.security.blacksyrius.com"
  "securityscanconfigs.security.blacksyrius.com"
)
for crd in "${CRDS[@]}"; do
  if $KUBECTL get crd "${crd}" &>/dev/null; then
    $KUBECTL delete crd "${crd}" --ignore-not-found && ok "Deleted CRD: ${crd}"
  else
    warn "CRD ${crd} not found — skipping"
  fi
done

echo ""
ok "Cleanup complete. Context '${KUBE_CONTEXT}' is now free of Korsair resources."
echo ""
echo -e "Next step: add this cluster as a remote target from your Kind operator:"
echo -e "  ${BOLD}./hack/add-cluster.sh ${KUBE_CONTEXT} ~/.kube/config${NC}"
echo ""
