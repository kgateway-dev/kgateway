#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

METALLB_VERSION=${METALLB_VERSION:-v0.13.7}
# The IP family of the cluster: ipv4, ipv6, or dual. Determines which of the
# docker network's subnets the LoadBalancer pool is carved out of.
IP_FAMILY="${IP_FAMILY:-ipv4}"
# The kind cluster to install into. Every kubectl call below is pinned to this
# cluster's context: this script also runs standalone via `make metallb`, where
# the current context may well belong to some other cluster.
CLUSTER_NAME="${CLUSTER_NAME:-kind}"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-${CLUSTER_NAME}}"

# Optional per-cluster pool overrides. Every kind cluster shares one docker
# network, so two clusters derive the same pool from it and advertise
# overlapping addresses; a second concurrent cluster needs its own slice.
# CLUSTER_SUBNET is a /24 prefix ("172.18.101") and CLUSTER_SUBNET_V6 a /64
# prefix ("fc00:f853:ccd:e793:f001"). Both have to sit inside the docker
# network's subnet, and outside the range kind hands to nodes, to be reachable
# from the host without colliding with anything.
CLUSTER_SUBNET="${CLUSTER_SUBNET:-}"
CLUSTER_SUBNET_V6="${CLUSTER_SUBNET_V6:-}"

echo "installing MetalLB ${METALLB_VERSION} on cluster ${CLUSTER_NAME} (context ${KUBE_CONTEXT}) with ipFamily=${IP_FAMILY}"

kubectl --context "$KUBE_CONTEXT" apply -f https://raw.githubusercontent.com/metallb/metallb/${METALLB_VERSION}/config/manifests/metallb-native.yaml

# Wait for MetalLB to become available.
kubectl --context "$KUBE_CONTEXT" rollout status -n metallb-system deployment/controller --timeout 5m
kubectl --context "$KUBE_CONTEXT" rollout status -n metallb-system daemonset/speaker --timeout 5m
kubectl --context "$KUBE_CONTEXT" wait -n metallb-system pod -l app=metallb --for=condition=Ready --timeout=60s

# Ready pods are not enough. The IPAddressPool and L2Advertisement CRs below go
# through MetalLB's own validating webhook, and that only starts admitting once
# the webhook Service has endpoints; applying before then fails with a
# connection error.
echo "waiting for the MetalLB admission webhook to have endpoints"
webhook_ready=false
for _ in $(seq 1 60); do
  if [[ -n "$(kubectl --context "$KUBE_CONTEXT" get endpointslices -n metallb-system \
      -l kubernetes.io/service-name=webhook-service \
      -o jsonpath='{.items[*].endpoints[*].addresses[0]}')" ]]; then
    webhook_ready=true
    break
  fi
  sleep 1
done
if [[ "$webhook_ready" != "true" ]]; then
  echo "MetalLB admission webhook never got endpoints; cannot configure the address pool" >&2
  exit 1
fi

NETWORK="${METALLB_NETWORK:-kind}"

# Carve a small pool out of the top of the docker network's subnet. kind never
# assigns node addresses from that range, so the pool cannot collide with them.
# Only the first subnet of each family is used; a network with several is not
# something this harness sets up.
function ipv4_pool() {
  local subnet
  if [[ -n "$CLUSTER_SUBNET" ]]; then
    echo "${CLUSTER_SUBNET}.1-${CLUSTER_SUBNET}.254"
    return
  fi
  subnet=$(docker network inspect "$NETWORK" | jq -r 'first(.[].IPAM.Config[].Subnet | select(contains(":") | not)) // empty' | cut -d '.' -f1,2)
  if [[ -z "$subnet" ]]; then
    echo "docker network '$NETWORK' has no IPv4 subnet; cannot build an IPv4 MetalLB pool" >&2
    return 1
  fi
  echo "${subnet}.255.0-${subnet}.255.231"
}

# The IPv6 equivalent. The pool has to sit inside the subnet, so it can only vary
# the host bits: for a /64 that is everything after the fourth hextet. The written
# form of the prefix may hold fewer than four hextets ("fd00:dead:beef::/64"), and
# appending to it directly would land outside the subnet, so pad it out first.
function ipv6_pool() {
  local subnet base prefixlen head parts n
  if [[ -n "$CLUSTER_SUBNET_V6" ]]; then
    echo "${CLUSTER_SUBNET_V6}::1-${CLUSTER_SUBNET_V6}::fffe"
    return
  fi
  subnet=$(docker network inspect "$NETWORK" | jq -r 'first(.[].IPAM.Config[].Subnet | select(contains(":"))) // empty')
  if [[ -z "$subnet" ]]; then
    echo "docker network '$NETWORK' has no IPv6 subnet; enable IPv6 on the docker daemon" >&2
    return 1
  fi

  base="${subnet%%/*}"
  prefixlen="${subnet#*/}"
  if [[ "$prefixlen" != "64" ]]; then
    echo "docker network '$NETWORK' has IPv6 subnet '$subnet'; only a /64 is supported here" >&2
    return 1
  fi

  head="${base%%::*}"
  IFS=':' read -ra parts <<< "$head"
  n=${#parts[@]}
  if (( n > 4 )); then
    echo "IPv6 subnet '$subnet' has more than four hextets before '::'; cannot derive a pool" >&2
    return 1
  fi
  while (( n < 4 )); do
    head="${head}:0"
    n=$((n + 1))
  done

  echo "${head}:ffff::0-${head}:ffff::e7"
}

# Assign to a scalar first: with several command substitutions in one statement
# the exit status is only the last one's, so a failed lookup would be swallowed
# and MetalLB would come up with a half-empty pool.
ADDRESSES=()
case "$IP_FAMILY" in
  ipv4)
    v4=$(ipv4_pool)
    ADDRESSES+=("$v4")
    ;;
  ipv6)
    v6=$(ipv6_pool)
    ADDRESSES+=("$v6")
    ;;
  dual)
    # Both families in the same pool: MetalLB only assigns two addresses to a
    # dual-stack Service when one pool holds a range of each family.
    v4=$(ipv4_pool)
    v6=$(ipv6_pool)
    ADDRESSES+=("$v4" "$v6")
    ;;
  *)
    echo "IP_FAMILY must be one of ipv4, ipv6, or dual; got '$IP_FAMILY'" >&2
    exit 1
    ;;
esac

echo "configuring MetalLB address pool: ${ADDRESSES[*]}"

ADDRESS_YAML=""
for addr in "${ADDRESSES[@]}"; do
  ADDRESS_YAML+="    - ${addr}"$'\n'
done

# Note: each line below must begin with one tab character; this is to get EOF working within
# an if block. The `-` in the `<<-EOF`` strips out the leading tab from each line, see
# https://tldp.org/LDP/abs/html/here-docs.html

# Apply the IPAddressPool on its own first: the L2Advertisement below names it,
# and the webhook rejects an advertisement whose pool does not exist yet.
kubectl --context "$KUBE_CONTEXT" apply -f - <<-EOF
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: address-pool
  namespace: metallb-system
spec:
  addresses:
${ADDRESS_YAML}
EOF

kubectl --context "$KUBE_CONTEXT" apply -f - <<-EOF
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: advertisement
  namespace: metallb-system
spec:
  ipAddressPools:
    - address-pool
EOF

echo "MetalLB installation completed for ${CLUSTER_NAME}"
