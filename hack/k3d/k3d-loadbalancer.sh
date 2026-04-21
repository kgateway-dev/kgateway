#!/usr/bin/env bash
# Lightweight LoadBalancer IP assigner for k3d clusters.
# Replaces MetalLB with a simple background process that assigns unique IPs
# from the Docker network to LoadBalancer services (~0s vs ~35s startup).
#
# How it works:
#   1. Watches for LoadBalancer services missing an external IP
#   2. Assigns a unique IP from the k3d Docker network subnet
#   3. Adds the IP to the k3d node's network interface (ARP reachability)
#   4. Patches the service status (kube-proxy creates iptables DNAT rules)
#
# Usage:
#   ./k3d-loadbalancer.sh <cluster-name>          # foreground
#   ./k3d-loadbalancer.sh <cluster-name> &         # background
#   K3D_LB_VERBOSE=1 ./k3d-loadbalancer.sh <name>  # debug output

set -euo pipefail

CLUSTER_NAME="${1:?usage: $0 <cluster-name>}"
VERBOSE="${K3D_LB_VERBOSE:-0}"

NODE="k3d-${CLUSTER_NAME}-server-0"
SUBNET_PREFIX=$(docker network inspect "k3d-${CLUSTER_NAME}" \
  | jq -r '.[].IPAM.Config[].Subnet | select(contains(":") | not)' \
  | cut -d'.' -f1,2)

log() { [[ "$VERBOSE" == "1" ]] && echo "[k3d-lb] $*" >&2 || true; }

# Scan existing LB services to find the next available IP counter.
# This handles restarts without assigning duplicate IPs.
init_counter() {
  local max=-1
  local existing
  existing=$(kubectl get svc -A \
    -o jsonpath='{range .items[?(@.spec.type=="LoadBalancer")]}{.status.loadBalancer.ingress[0].ip}{"\n"}{end}' 2>/dev/null || true)
  while read -r ip; do
    [[ -z "$ip" ]] && continue
    # Extract the last octet from IPs in our range (SUBNET_PREFIX.255.N)
    if [[ "$ip" == "${SUBNET_PREFIX}.255."* ]]; then
      local n="${ip##*.}"
      (( n > max )) && max=$n
    fi
  done <<< "$existing"
  echo $((max + 1))
}

COUNTER=$(init_counter)
log "watching cluster=${CLUSTER_NAME} node=${NODE} subnet=${SUBNET_PREFIX}.255.x counter=${COUNTER}"

while true; do
  # List all LB services with their current ingress IP (empty if pending)
  SVCS=$(kubectl get svc -A \
    -o jsonpath='{range .items[?(@.spec.type=="LoadBalancer")]}{.metadata.namespace},{.metadata.name},{.status.loadBalancer.ingress[0].ip}{"\n"}{end}' 2>/dev/null || true)

  while IFS=, read -r ns name ip; do
    [[ -z "$name" ]] && continue
    [[ -n "$ip" ]] && continue

    ip="${SUBNET_PREFIX}.255.${COUNTER}"
    COUNTER=$((COUNTER + 1))

    # Add IP to the node's interface so the node responds to ARP
    docker exec "$NODE" ip addr add "${ip}/32" dev eth0 2>/dev/null || true

    # Patch the service status so kube-proxy creates iptables rules
    if kubectl patch svc "$name" -n "$ns" --subresource=status --type=merge \
      -p "{\"status\":{\"loadBalancer\":{\"ingress\":[{\"ip\":\"${ip}\"}]}}}" 2>/dev/null; then
      log "assigned ${ip} -> ${ns}/${name}"
    fi
  done <<< "$SVCS"

  sleep 0.5
done
