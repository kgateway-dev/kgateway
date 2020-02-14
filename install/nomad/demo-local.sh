#!/usr/bin/env bash

# Will exit script if we would use an uninitialised variable (nounset) or when a
# simple command (not a control structure) fails (errexit)
set -eu

trap 'kill $(jobs -p)' EXIT

# Get directory this script is located in to access script local files
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"

echo "Launching Consul-Vault-Nomad for Demo (see ${SCRIPT_DIR}/.hashi-logs for logs)"
("${SCRIPT_DIR}/launch-consul-vault-nomad-dev.sh" >"${SCRIPT_DIR}/.hashi-logs" 2>&1) &

sleep 10

if ! [[ -x $(command -v levant) ]]; then
  printf '\nYou must install levant first\n\n'
  exit
fi

VARFILE="${SCRIPT_DIR}/variables.yaml"

printf '\nDeploying Gloo\n\n'
levant deploy \
  -var-file="${VARFILE}" \
  jobs/gloo.nomad

printf '\nDeploying Petstore\n\n'
levant deploy \
  -var-file="${VARFILE}" \
  jobs/petstore.nomad

FAIL=0

printf '\nAdding Gateway Proxies\n\n'

consul kv put gloo/gateway.solo.io/v1/Gateway/gloo-system/gateway-proxy - <<-EOF
bindAddress: '::'
bindPort: 8080
httpGateway: {}
metadata:
  labels:
    app: gloo
  name: gateway-proxy
  namespace: gloo-system
  resourceVersion: "300"
proxyNames:
- gateway-proxy
status:
  reportedBy: gateway
  state: Accepted
  subresourceStatuses:
    '*v1.Proxy.gloo-system.gateway-proxy':
      reportedBy: gloo
      state: Accepted
useProxyProto: false
EOF

consul kv put gloo/gateway.solo.io/v1/Gateway/gloo-system/gateway-proxy-ssl - <<-EOF
bindAddress: '::'
bindPort: 8443
httpGateway: {}
metadata:
  labels:
    app: gloo
  name: gateway-proxy-ssl
  namespace: gloo-system
  resourceVersion: "292"
proxyNames:
- gateway-proxy
ssl: true
status:
  reportedBy: gateway
  state: Accepted
  subresourceStatuses:
    '*v1.Proxy.gloo-system.gateway-proxy':
      reportedBy: gloo
      state: Accepted
useProxyProto: false
EOF

printf '\nAdding route to Petstore\n\n'
glooctl add route \
  --path-prefix='/' \
  --dest-name='petstore' \
  --prefix-rewrite='/api/pets' \
  --use-consul

sleep 5

INGRESS_IP='localhost'
DOCKER_HOST='localhost'
if [[ "${OSTYPE}" == "linux-gnu" ]]; then
  DOCKER_HOST='172.17.0.1'
  INGRESS_IP='172.17.0.1'
elif [[ "${OSTYPE}" == "darwin"* ]]; then
  DOCKER_HOST='host.docker.internal'
fi

printf '\ncURL the gateway\n\n'
curl "${INGRESS_IP}:8080/"

printf '\nCtrl+C to exit.\n\n'

for job in $(jobs -p); do
  echo "${job}"
  wait "${job}" || ((FAIL++))
done

echo "${FAIL} background processes failed"
