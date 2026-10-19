#!/usr/bin/env bash

set -euo pipefail

# Checks the controller's xDS invariant counters after a cluster test run
# (conformance, e2e or manual). Any nonzero sample is a kgateway bug:
#   - kgateway_xds_snapshot_perclient_inconsistent_snapshots_total: a published
#     snapshot had an unreferenced EDS/RDS resource or a listener naming a
#     missing RouteConfiguration. Recorded only with
#     KGW_XDS_SNAPSHOT_CONSISTENCY_CHECK=true; the script warns otherwise.
#   - kgateway_envoy_xds_rejects_total: a client NACKed a published response.
#
# Environment: INSTALL_NAMESPACE (default kgateway-system), KUBE_CONTEXT (optional).
# Exit codes: 0 no violations, 1 violations recorded, 2 metrics unreachable.

INSTALL_NAMESPACE="${INSTALL_NAMESPACE:-kgateway-system}"
KUBECTL=(kubectl)
if [[ -n "${KUBE_CONTEXT:-}" ]]; then
    KUBECTL+=(--context "${KUBE_CONTEXT}")
fi

consistency_check_enabled() {
    "${KUBECTL[@]}" -n "${INSTALL_NAMESPACE}" get deploy kgateway \
        -o jsonpath='{.spec.template.spec.containers[*].env[?(@.name=="KGW_XDS_SNAPSHOT_CONSISTENCY_CHECK")].value}' 2>/dev/null || true
}

# Scrape the controller metrics endpoint via a short-lived port-forward.
PORT=19092
"${KUBECTL[@]}" -n "${INSTALL_NAMESPACE}" port-forward deploy/kgateway "${PORT}:9092" >/dev/null 2>&1 &
PF_PID=$!
trap 'kill "${PF_PID}" 2>/dev/null || true' EXIT

METRICS=""
for _ in $(seq 1 20); do
    if METRICS=$(curl -sf "localhost:${PORT}/metrics" 2>/dev/null); then
        break
    fi
    sleep 0.5
done

if [[ -z "${METRICS}" ]]; then
    echo "ERROR: could not scrape controller metrics from deploy/kgateway in ${INSTALL_NAMESPACE}" >&2
    exit 2
fi

FAILED=0

# Both counters only materialize on the first violation; absence passes.
check_metric() {
    local metric="$1" description="$2"
    local bad
    bad=$(echo "${METRICS}" | awk -v m="${metric}" '$0 ~ "^"m && $NF != "0" {print}')
    if [[ -n "${bad}" ]]; then
        echo "ERROR: ${description}" >&2
        echo "       This is a kgateway bug; please report it with the controller's logs." >&2
        echo "${bad}" >&2
        FAILED=1
    fi
}

check_metric "kgateway_envoy_xds_rejects_total" \
    "connected clients NACKed published xDS responses (they are serving older config than published)."

if [[ "$(consistency_check_enabled)" == "true" ]]; then
    check_metric "kgateway_xds_snapshot_perclient_inconsistent_snapshots_total" \
        "the controller published inconsistent per-client xDS snapshots (see the controller's error log)."
else
    echo "WARNING: KGW_XDS_SNAPSHOT_CONSISTENCY_CHECK is not enabled on deploy/kgateway in ${INSTALL_NAMESPACE};" >&2
    echo "         snapshot consistency was not recorded and only NACKs were verified." >&2
fi

if [[ "${FAILED}" -ne 0 ]]; then
    exit 1
fi

echo "OK: no xDS invariant violations were recorded."
