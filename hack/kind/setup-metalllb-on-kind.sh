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
function ipv4_pool() {
  local subnet
  subnet=$(docker network inspect "$NETWORK" | jq -r '.[].IPAM.Config[].Subnet | select(contains(":") | not)' | cut -d '.' -f1,2)
  if [[ -z "$subnet" ]]; then
    echo "docker network '$NETWORK' has no IPv4 subnet; cannot build an IPv4 MetalLB pool" >&2
    return 1
  fi
  echo "${subnet}.255.0-${subnet}.255.231"
}

# The IPv6 equivalent. kind's default IPv6 subnet is a /64, so the pool is taken
# from a high prefix within it that node allocation does not reach.
function ipv6_pool() {
  local subnet prefix
  subnet=$(docker network inspect "$NETWORK" | jq -r '.[].IPAM.Config[].Subnet | select(contains(":"))')
  if [[ -z "$subnet" ]]; then
    echo "docker network '$NETWORK' has no IPv6 subnet; enable IPv6 on the docker daemon" >&2
    return 1
  fi
  prefix="${subnet%::*}"
  echo "${prefix}:ffff::0-${prefix}:ffff::e7"
}

ADDRESSES=()
case "$IP_FAMILY" in
  ipv4) ADDRESSES+=("$(ipv4_pool)") ;;
  ipv6) ADDRESSES+=("$(ipv6_pool)") ;;
  dual) ADDRESSES+=("$(ipv4_pool)" "$(ipv6_pool)") ;;
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
