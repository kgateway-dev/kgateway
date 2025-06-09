#! /bin/bash

set -o errexit
set -o pipefail
set -o nounset

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
HELM="${HELM:-go tool helm}"

function install_localstack() {
  # Install localstack
  $HELM repo add localstack-repo https://helm.localstack.cloud
  $HELM repo update

  $HELM upgrade -i --create-namespace localstack localstack-repo/localstack --namespace localstack -f ${ROOT_DIR}/localstack-values.yaml
  kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=localstack -n localstack --timeout=120s
}

function get_localstack_endpoint() {
  localstack_port=$(kubectl get -n localstack svc/localstack -o jsonpath="{.spec.ports[0].nodePort}")
  localstack_host=$(kubectl get nodes -o jsonpath="{.items[0].status.addresses[0].address}")
  echo "http://${localstack_host}:${localstack_port}"
}

install_localstack
localstack_endpoint=$(get_localstack_endpoint)

echo "Localstack endpoint: ${localstack_endpoint}"
echo "export ENDPOINT=${localstack_endpoint}"
