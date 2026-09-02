#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

METALLB_VERSION=${METALLB_VERSION:-v0.13.7}
# The IP family of the cluster: ipv4, ipv6, or dual. Determines which of the
# docker network's subnets the LoadBalancer pool is carved out of.
IP_FAMILY="${IP_FAMILY:-ipv4}"

kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/${METALLB_VERSION}/config/manifests/metallb-native.yaml

# Wait for MetalLB to become available.
kubectl rollout status -n metallb-system deployment/controller --timeout 5m
kubectl rollout status -n metallb-system daemonset/speaker --timeout 5m
kubectl wait -n metallb-system  pod -l app=metallb --for=condition=Ready --timeout=10s

NETWORK="${METALLB_NETWORK:-kind}"

# Carve a small pool out of the top of the docker network's subnet. kind never
# assigns node addresses from that range, so the pool cannot collide with them.
# Only the first subnet of each family is used; a network with several is not
# something this harness sets up.
function ipv4_pool() {
  local subnet
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
    v4=$(ipv4_pool)
    v6=$(ipv6_pool)
    ADDRESSES+=("$v4" "$v6")
    ;;
  *)
    echo "IP_FAMILY must be one of ipv4, ipv6, or dual; got '$IP_FAMILY'" >&2
    exit 1
    ;;
esac

ADDRESS_YAML=""
for addr in "${ADDRESSES[@]}"; do
  ADDRESS_YAML+="    - ${addr}"$'\n'
done

# Note: each line below must begin with one tab character; this is to get EOF working within
# an if block. The `-` in the `<<-EOF`` strips out the leading tab from each line, see
# https://tldp.org/LDP/abs/html/here-docs.html
kubectl apply -f - <<-EOF
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: address-pool
  namespace: metallb-system
spec:
  addresses:
${ADDRESS_YAML}
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: advertisement
  namespace: metallb-system
spec:
  ipAddressPools:
    - address-pool
EOF
