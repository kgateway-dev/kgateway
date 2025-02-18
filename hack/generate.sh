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

echo "Generating clientset at ${OUTPUT_PKG}/${CLIENTSET_PKG_NAME} for versions: ${VERSIONS[@]}"

API_INPUT_DIRS_SPACE=""
API_INPUT_DIRS_COMMA=""
for VERSION in "${VERSIONS[@]}"
do
  API_INPUT_DIRS_SPACE+="${APIS_PKG}/api/${VERSION} "
  API_INPUT_DIRS_COMMA+="${APIS_PKG}/api/${VERSION},"
done
API_INPUT_DIRS_SPACE="${API_INPUT_DIRS_SPACE%,}" # drop trailing space
API_INPUT_DIRS_COMMA="${API_INPUT_DIRS_COMMA%,}" # drop trailing comma

go run k8s.io/code-generator/cmd/register-gen --output-file zz_generated.register.go ${API_INPUT_DIRS_SPACE}
go run sigs.k8s.io/controller-tools/cmd/controller-gen crd:maxDescLen=0 object rbac:roleName=kgateway paths="${APIS_PKG}/api/${VERSION}" \
    output:crd:artifacts:config=${ROOT_DIR}/install/helm/kgateway/crds/ output:rbac:artifacts:config=${ROOT_DIR}/install/helm/kgateway/templates

# throw away
new_report="$(mktemp -t "$(basename "$0").api_violations.XXXXXX")"

go run k8s.io/kube-openapi/cmd/openapi-gen \
  --output-file zz_generated.openapi.go \
  --report-filename "${new_report}" \
  --output-dir "${ROOT_DIR}/pkg/generated/openapi" \
  --output-pkg "github.com/kgateway-dev/kgateway/v2/pkg/generated/openapi" \
  $API_INPUT_DIRS_SPACE \
  sigs.k8s.io/gateway-api/apis/v1 \
  k8s.io/apimachinery/pkg/apis/meta/v1 \
  k8s.io/api/core/v1 \
  k8s.io/apimachinery/pkg/runtime \
  k8s.io/apimachinery/pkg/util/intstr \
  k8s.io/apimachinery/pkg/api/resource \
  k8s.io/apimachinery/pkg/version

go run k8s.io/code-generator/cmd/applyconfiguration-gen \
  --openapi-schema <(go run ${ROOT_DIR}/cmd/modelschema) \
  --output-dir "${ROOT_DIR}/api/applyconfiguration" \
  --output-pkg "github.com/kgateway-dev/kgateway/v2/api/applyconfiguration" \
  ${API_INPUT_DIRS_SPACE}

go run k8s.io/code-generator/cmd/client-gen \
  --clientset-name "versioned" \
  --input-base "${APIS_PKG}" \
  --input "${API_INPUT_DIRS_COMMA//${APIS_PKG}/}" \
  --output-dir "${ROOT_DIR}/pkg/client/${CLIENTSET_PKG_NAME}" \
  --output-pkg "${OUTPUT_PKG}/${CLIENTSET_PKG_NAME}" \
  --apply-configuration-package "${APIS_PKG}/api/applyconfiguration"

# fix imports of gen code
go run golang.org/x/tools/cmd/goimports -w ${ROOT_DIR}/pkg/client
go run golang.org/x/tools/cmd/goimports -w ${ROOT_DIR}/api
