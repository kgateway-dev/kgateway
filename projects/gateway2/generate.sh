#!/bin/bash

# Based of of gateway-api codegen (https://github.com/kubernetes-sigs/gateway-api/blob/main/hack/update-codegen.sh)
# generate deep copy and clients for our api.
# In this project, clients mostly used as fakes for testing.

set -e
set -x

APIS_PKG=""

readonly SCRIPT_ROOT="$(cd "$(dirname "${BASH_SOURCE}")" && pwd)"
readonly OUTPUT_PKG=github.com/solo-io/gloo/projects/gateway2/pkg/client
readonly APIS_PKG=github.com/solo-io/gloo/projects/gateway2
readonly CLIENTSET_NAME=versioned
readonly CLIENTSET_PKG_NAME=clientset
readonly VERSIONS=(v1alpha1)

echo "Generating clientset at ${OUTPUT_PKG}/${CLIENTSET_PKG_NAME} for versions: ${VERSIONS[@]}"
echo "Running from directory: ${SCRIPT_ROOT}"


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
go run sigs.k8s.io/controller-tools/cmd/controller-gen crd:maxDescLen=0 object rbac:roleName=k8sgw-controller paths="${APIS_PKG}/api/${VERSION}" \
    output:crd:artifacts:config=${SCRIPT_ROOT}/../../install/helm/gloo/crds/ output:rbac:artifacts:config=${SCRIPT_ROOT}/../../install/helm/gloo/files/rbac


# throw away
new_report="$(mktemp -t "$(basename "$0").api_violations.XXXXXX")"

go run k8s.io/kube-openapi/cmd/openapi-gen \
  --output-file zz_generated.openapi.go \
  --report-filename "${new_report}" \
  --output-dir "pkg/generated/openapi" \
  --output-pkg "${APIS_PKG}/pkg/generated/openapi" \
  ${COMMON_FLAGS} \
  $API_INPUT_DIRS_SPACE \
  sigs.k8s.io/gateway-api/apis/v1 \
  k8s.io/apimachinery/pkg/apis/meta/v1 \
  k8s.io/api/core/v1 \
  k8s.io/apimachinery/pkg/runtime \
  k8s.io/apimachinery/pkg/util/intstr \
  k8s.io/apimachinery/pkg/api/resource \
  k8s.io/apimachinery/pkg/version

go run k8s.io/code-generator/cmd/applyconfiguration-gen \
  --openapi-schema <(go run ${SCRIPT_ROOT}/cmd/modelschema) \
  --output-dir "api/applyconfiguration" \
  --output-pkg "${APIS_PKG}/api/applyconfiguration" \
  ${COMMON_FLAGS} \
  ${API_INPUT_DIRS_SPACE}

go run k8s.io/code-generator/cmd/client-gen \
  --clientset-name "versioned" \
  --input-base "${APIS_PKG}" \
  --input "${API_INPUT_DIRS_COMMA//${APIS_PKG}/}" \
  --output-dir "pkg/client/${CLIENTSET_PKG_NAME}" \
  --output-pkg "${OUTPUT_PKG}/${CLIENTSET_PKG_NAME}" \
  --apply-configuration-package "${APIS_PKG}/api/applyconfiguration" \
  ${COMMON_FLAGS}

# fix imports of gen code
go run golang.org/x/tools/cmd/goimports -w ${SCRIPT_ROOT}/api/v1alpha1