#!/usr/bin/env bash

GLOO_VERSION='1.3.1'

trap 'kill $(jobs -p)' EXIT

# Get directory this script is located in to access script local files
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"

if [[ "${OSTYPE}" == "darwin"* ]]; then
  # need to install and run Weave Net https://www.weave.works/docs/net/latest/install/installing-weave/
  if [[ -x $(command -v weave) ]]; then
    WEAVE_CMD="$(command -v weave)"
  else
    curl -L git.io/weave -o "${SCRIPT_DIR}/weave"
    chmod a+x "${SCRIPT_DIR}/weave"
    WEAVE_CMD="${SCRIPT_DIR}/weave"
  fi

  "${WEAVE_CMD}" stop
  "${WEAVE_CMD}" launch || true # ignore errors as 2nd+ call returns error saying already running
fi

VARFILE="${SCRIPT_DIR}/variables.yaml"

INGRESS_IP='localhost'
DOCKER_HOST='localhost'
if [[ "${OSTYPE}" == "linux-gnu" ]]; then
  DOCKER_HOST='172.17.0.1'
  INGRESS_IP='172.17.0.1'
elif [[ "${OSTYPE}" == "darwin"* ]]; then
  DOCKER_HOST='host.docker.internal'
fi

cat >"${SCRIPT_DIR}/variables.yaml" <<EOF
datacenter: dc1

config:
  # the "namespace" where Gloo will read/write configuration
  # change this for multiple installations of Gloo
  namespace: gloo-system
  # the rate to poll Vault for secrets
  # maximum wait time on blocking requests to Consul
  refreshRate: 30s

consul:
  address: ${DOCKER_HOST}:8500

vault:
  address: http://${DOCKER_HOST}:8200
  token: root

gloo:
  # the port where Gloo serves config to Envoy
  xdsPort: 9977
  image:
    registry: quay.io/solo-io
    repository: gloo
    tag: ${GLOO_VERSION}
  cpuLimit: 1000
  memLimit: 500
  bandwidthLimit: 10
  # number of instances of gloo config server
  replicas: 1

discovery:
  image:
    registry: quay.io/solo-io
    repository: discovery
    tag: ${GLOO_VERSION}
  cpuLimit: 500
  memLimit: 500
  bandwidthLimit: 10

gateway:
  image:
    registry: quay.io/solo-io
    repository: gateway
    tag: ${GLOO_VERSION}
  cpuLimit: 250
  memLimit: 250
  bandwidthLimit: 5

gatewayProxy:
  image:
    registry: quay.io/solo-io
    repository: gloo-envoy-wrapper
    tag: ${GLOO_VERSION}
  cpuLimit: 500
  memLimit: 500
  bandwidthLimit: 100
  # number of instances of gateway proxy
  replicas: 1
  httpPort: 8080
  httpsPort: 8443
  adminPort: 19000
  # expose the http and https ports
  # on the host machine
  exposePorts: true
EOF

if [[ "${OSTYPE}" == "darwin"* ]]; then
  cat >>"${SCRIPT_DIR}/variables.yaml" <<EOF

# use this network plugin when running on mac
dockerNetwork: weave
EOF
fi

consul agent -dev --client='0.0.0.0' &

sleep 5

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

vault server -dev -dev-root-token-id='root' \
  -log-level='trace' \
  -dev-listen-address='0.0.0.0:8200' &

sleep 1

VAULT_ADDR='http://127.0.0.1:8200' VAULT_TOKEN='root' vault policy write gloo ./gloo-policy.hcl

LINUX_ARGS=
if [[ "${OSTYPE}" == "linux-gnu" ]]; then
  LINUX_ARGS=--network-interface='docker0'
fi

nomad agent -dev \
  --bind='0.0.0.0' ${LINUX_ARGS} \
  --vault-enabled='true' \
  --vault-address='http://127.0.0.1:8200' \
  --vault-token='root' &

FAIL=0

for job in $(jobs -p); do
  echo "${job}"
  wait "${job}" || ((FAIL++))
done

echo "${FAIL} failed"
