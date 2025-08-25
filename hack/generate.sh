#!/bin/bash

# Based of of gateway-api codegen (https://github.com/kubernetes-sigs/gateway-api/blob/main/hack/update-codegen.sh)
# generate deep copy and clients for our api.
# In this project, clients mostly used as fakes for testing.

set -o errexit
set -o nounset
set -o pipefail

set -x

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE}")"/.. && pwd)"
readonly OUTPUT_PKG=github.com/kgateway-dev/kgateway/v2/pkg/client
readonly APIS_PKG=github.com/kgateway-dev/kgateway/v2
readonly CLIENTSET_NAME=versioned
readonly CLIENTSET_PKG_NAME=clientset
readonly VERSIONS=( v1alpha1 )

# well known dirs for codegen, should be cleaned before fresh gen
readonly OPENAPI_GEN_DIR=pkg/generated/openapi
readonly APPLY_CFG_DIR=api/applyconfiguration
readonly CLIENT_GEN_DIR=pkg/client
readonly CRD_DIR=install/helm/kgateway-crds/templates
# manifests dir only used for outputting rbac artifacts and existing file will be overwritten so no need to clean
readonly MANIFESTS_DIR=install/helm/kgateway/templates

echo "Generating clientset at ${OUTPUT_PKG}/${CLIENTSET_PKG_NAME} for versions:" "${VERSIONS[@]}"

API_INPUT_DIRS_SPACE=""
API_INPUT_DIRS_COMMA=""
for VERSION in "${VERSIONS[@]}"
do
  API_INPUT_DIRS_SPACE+="${APIS_PKG}/api/${VERSION} "
  API_INPUT_DIRS_COMMA+="${APIS_PKG}/api/${VERSION},"
done
API_INPUT_DIRS_SPACE="${API_INPUT_DIRS_SPACE%,}" # drop trailing space
API_INPUT_DIRS_COMMA="${API_INPUT_DIRS_COMMA%,}" # drop trailing comma

go tool register-gen --output-file zz_generated.register.go ${API_INPUT_DIRS_SPACE}
go tool controller-gen crd:maxDescLen=0 object rbac:roleName=kgateway paths="${APIS_PKG}/api/${VERSION}" \
    output:crd:artifacts:config=${ROOT_DIR}/${CRD_DIR} output:rbac:artifacts:config=${ROOT_DIR}/${MANIFESTS_DIR}
# Template the ClusterRole name to include the namespace
if [[ "$OSTYPE" == "darwin"* ]]; then
  # On macOS, prefer gsed (GNU sed) if available
  if command -v gsed &> /dev/null; then
    gsed -i 's/name: kgateway/name: kgateway-{{ .Release.Namespace }}/g' "${ROOT_DIR}/${MANIFESTS_DIR}/role.yaml"
  else
    # Fallback to macOS's native sed
    sed -i '' 's/name: kgateway/name: kgateway-{{ .Release.Namespace }}/g' "${ROOT_DIR}/${MANIFESTS_DIR}/role.yaml"
  fi
else
  # For other OSes like Linux
  sed -i 's/name: kgateway/name: kgateway-{{ .Release.Namespace }}/g' "${ROOT_DIR}/${MANIFESTS_DIR}/role.yaml"
fi


# throw away
new_report="$(mktemp -t "$(basename "$0").api_violations.XXXXXX")"

go tool openapi-gen \
  --output-file zz_generated.openapi.go \
  --report-filename "${new_report}" \
  --output-dir "${ROOT_DIR}/${OPENAPI_GEN_DIR}" \
  --output-pkg "github.com/kgateway-dev/kgateway/v2/pkg/generated/openapi" \
  $API_INPUT_DIRS_SPACE \
  sigs.k8s.io/gateway-api/apis/v1 \
  sigs.k8s.io/gateway-api/apis/v1alpha2 \
  k8s.io/apimachinery/pkg/apis/meta/v1 \
  k8s.io/api/core/v1 \
  k8s.io/apimachinery/pkg/runtime \
  k8s.io/apimachinery/pkg/util/intstr \
  k8s.io/apimachinery/pkg/api/resource \
  k8s.io/apimachinery/pkg/version

go tool applyconfiguration-gen \
  --openapi-schema <(go run ${ROOT_DIR}/cmd/modelschema) \
  --output-dir "${ROOT_DIR}/${APPLY_CFG_DIR}" \
  --output-pkg "github.com/kgateway-dev/kgateway/v2/api/applyconfiguration" \
  ${API_INPUT_DIRS_SPACE}

go tool client-gen \
  --clientset-name "versioned" \
  --input-base "${APIS_PKG}" \
  --input "${API_INPUT_DIRS_COMMA//${APIS_PKG}/}" \
  --output-dir "${ROOT_DIR}/${CLIENT_GEN_DIR}/${CLIENTSET_PKG_NAME}" \
  --output-pkg "${OUTPUT_PKG}/${CLIENTSET_PKG_NAME}" \
  --apply-configuration-package "${APIS_PKG}/api/applyconfiguration"

go generate ${ROOT_DIR}/internal/...
go generate ${ROOT_DIR}/pkg/...

# fix imports of gen code
go tool goimports -w ${ROOT_DIR}/${CLIENT_GEN_DIR}
go tool goimports -w ${ROOT_DIR}/api
