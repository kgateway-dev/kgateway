#!/usr/bin/env bash
# xds-fleet-ab.sh - run the XdsFleet capacity ladder over two or more arms and
# emit one JSONL file the arms can be diffed from.
#
# An arm is a (helm chart, image tag, label) triple plus whatever environment
# the arm is meant to vary. Comparing builds this way - one arm per build, same
# ladder, same cluster, drained in between - is the only comparison the fleet
# numbers support: heap flattens against the memory cap, so a single heap
# reading does not discriminate between a build that is comfortable and one
# that is about to die. The client count an arm survives does.
#
# Usage:
#   hack/xds-fleet-ab.sh <out.jsonl> <arm> [<arm> ...]
#
# where each <arm> is a colon-separated
#   <label>:<chart-dir>:<image-tag>[:<KEY=VALUE,KEY=VALUE>]
# and the last field is optional controller environment for that arm.
#
# Example - one PR against its own base, at two fleet shapes:
#   hack/xds-fleet-ab.sh /tmp/ab.jsonl \
#     base:/path/to/base/install/helm/kgateway:base-ci1 \
#     mypr:/path/to/pr/install/helm/kgateway:pr-ci1
#
# Example - one build, one setting flipped:
#   hack/xds-fleet-ab.sh /tmp/ab.jsonl \
#     off:$PWD/install/helm/kgateway:my-ci1:KGW_XDS_SHARE_IDENTICAL_RESOURCES=false \
#     on:$PWD/install/helm/kgateway:my-ci1:KGW_XDS_SHARE_IDENTICAL_RESOURCES=true
#
# Fleet shape comes from the KGW_FLEET_* environment documented in
# test/e2e/features/loadtesting/README.md; the defaults below are the
# production shape this suite was built for.
set -uo pipefail

REPO_ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
CLUSTER_NAME=${CLUSTER_NAME:-xdsbench}
KUBE_CONTEXT=${KUBE_CONTEXT:-kind-${CLUSTER_NAME}}
INSTALL_NAMESPACE=${INSTALL_NAMESPACE:-kgateway-system}
IMAGE_REGISTRY=${IMAGE_REGISTRY:-ghcr.io/kgateway-dev}
LOG_DIR=${LOG_DIR:-${TMPDIR:-/tmp}}

# Production shape: 400 Gateways x 2 proxies over 8 waves is 50 Gateways per
# wave, which is the resolution needed to locate a capacity cliff rather than
# just observe that one happened.
export KGW_FLEET_SERVICES=${KGW_FLEET_SERVICES:-6000}
export KGW_FLEET_GATEWAYS=${KGW_FLEET_GATEWAYS:-400}
export KGW_FLEET_INLINE_BACKENDS=${KGW_FLEET_INLINE_BACKENDS:-20}
export KGW_FLEET_STREAMS_PER_GATEWAY=${KGW_FLEET_STREAMS_PER_GATEWAY:-2}
export KGW_FLEET_ZONES=${KGW_FLEET_ZONES:-3}
export KGW_FLEET_POD_LOCALITY=${KGW_FLEET_POD_LOCALITY:-true}
export KGW_FLEET_WAVES=${KGW_FLEET_WAVES:-8}
export KGW_FLEET_ITERATIONS=${KGW_FLEET_ITERATIONS:-4}
export KGW_FLEET_SETTLE_MS=${KGW_FLEET_SETTLE_MS:-5000}
export KGW_FLEET_MEMORY_LIMIT=${KGW_FLEET_MEMORY_LIMIT:-8Gi}
export KGW_FLEET_WAVE_TIMEOUT_SECONDS=${KGW_FLEET_WAVE_TIMEOUT_SECONDS:-1500}
export KGW_FLEET_ITERATION_TIMEOUT_SECONDS=${KGW_FLEET_ITERATION_TIMEOUT_SECONDS:-900}
export KGW_BENCH_VALIDATION=${KGW_BENCH_VALIDATION:-STANDARD}

usage() {
  sed -n '2,32p' < "${BASH_SOURCE[0]}" >&2
  exit 2
}

# drain removes everything the previous arm left behind. Both halves matter.
# Fake proxy pods are bound to fake Nodes and so have no kubelet to confirm
# deletion: without --force the namespace hangs in Terminating and the next arm
# fails to install. Nodes are cluster-scoped, so a leaked one silently supplies
# a stale zone label to the next arm and quietly invalidates it.
drain() {
  local ns node left
  for ns in $(kubectl --context "$KUBE_CONTEXT" get ns --no-headers 2>/dev/null | awk '$1 ~ /^kgw-fleet-/ {print $1}'); do
    kubectl --context "$KUBE_CONTEXT" delete pods -n "$ns" --all --force --grace-period=0 --wait=false >/dev/null 2>&1
    kubectl --context "$KUBE_CONTEXT" delete ns "$ns" --wait=false >/dev/null 2>&1
  done
  for node in $(kubectl --context "$KUBE_CONTEXT" get nodes --no-headers 2>/dev/null | awk '$1 ~ /node-[0-9]+$/ && $1 !~ /control-plane/ {print $1}'); do
    kubectl --context "$KUBE_CONTEXT" delete node "$node" >/dev/null 2>&1
  done
  for _ in $(seq 1 60); do
    left=$(kubectl --context "$KUBE_CONTEXT" get ns --no-headers 2>/dev/null | grep -c '^kgw-fleet-')
    [ "$left" = "0" ] && return 0
    sleep 10
  done
  echo "  WARNING: fleet namespaces still terminating after 10m" >&2
}

run_arm() {
  local label=$1 chart=$2 tag=$3 extra=${4:-}
  local log="$LOG_DIR/xdsfleet_${label}.log"

  drain
  if ! helm upgrade --install kgateway "$chart" \
      --kube-context "$KUBE_CONTEXT" --namespace "$INSTALL_NAMESPACE" \
      --set image.tag="$tag" --set image.registry="$IMAGE_REGISTRY" \
      --set validation.level="$(echo "$KGW_BENCH_VALIDATION" | tr '[:upper:]' '[:lower:]')" \
      --wait --timeout 7m >/dev/null 2>&1; then
    echo "=== $label: helm install FAILED, skipping arm" >&2
    return 1
  fi
  kubectl --context "$KUBE_CONTEXT" -n "$INSTALL_NAMESPACE" \
    rollout status deploy/kgateway --timeout=300s >/dev/null 2>&1

  echo "=== $label (tag=$tag services=$KGW_FLEET_SERVICES gateways=$KGW_FLEET_GATEWAYS extra=${extra:-none})"
  # -count=1 is load-bearing: go test caches a successful run and will
  # otherwise replay the previous arm's numbers under this arm's environment.
  SKIP_INSTALL=true KGW_ENABLE_XDS_FLEET=true \
  CLUSTER_NAME="$CLUSTER_NAME" INSTALL_NAMESPACE="$INSTALL_NAMESPACE" \
  KGW_BENCH_LABEL="$label" KGW_FLEET_EXTRA_ENV="$extra" KGW_BENCH_OUT="$OUT" \
    go test -C "$REPO_ROOT" -tags=e2e -v -count=1 -timeout 120m \
      ./test/e2e/tests -run '^TestKgateway$/^XdsFleet$' > "$log" 2>&1
  echo "  exit=$? waves=$(grep -c xds_fleet_wave "$log" 2>/dev/null) log=$log"
}

[ $# -ge 2 ] || usage
OUT=$1; shift
: > "$OUT"

for arm in "$@"; do
  IFS=: read -r label chart tag extra <<<"$arm"
  if [ -z "${label:-}" ] || [ -z "${chart:-}" ] || [ -z "${tag:-}" ]; then
    echo "malformed arm: $arm" >&2
    usage
  fi
  run_arm "$label" "$chart" "$tag" "${extra:-}"
done
drain

echo
echo "=== ladder ($OUT)"
sed -e 's/^xds_fleet_[a-z]* //' < "$OUT" | jq -r '
  if .died_at_clients then
    "\(.build)\tDIED at \(.died_at_clients) clients"
  else
    "\(.build)\t\(.clients) clients\t\(.heap_inuse_mb|floor)MB heap\t\(.rss_mb|floor)MB rss\t\(.cpu_seconds)s cpu\t\((.xds_resources/.clients)|floor) res/client"
  end' 2>/dev/null || cat < "$OUT"
