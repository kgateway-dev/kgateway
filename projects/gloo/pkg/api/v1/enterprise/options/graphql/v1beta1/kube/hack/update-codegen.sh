
#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail
set +x

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
ROOT_PKG=github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/graphql/v1beta1
CLIENT_PKG=${ROOT_PKG}/kube/client
APIS_PKG=${ROOT_PKG}/kube/apis

# Below code is copied from https://github.com/weaveworks/flagger/blob/master/hack/update-codegen.sh
CODEGEN_PKG=$(go list -f '{{ .Dir }}' -m k8s.io/code-generator)


echo ">> Using ${CODEGEN_PKG}"

# code-generator does work with go.mod but makes assumptions about
# the project living in $GOPATH/src. To work around this and support
# any location; create a temporary directory, use this as an output
# base, and copy everything back once generated.
TEMP_DIR=$(mktemp -d)
cleanup() {
    echo ">> Removing ${TEMP_DIR}"
    rm -rf ${TEMP_DIR}
}
trap "cleanup" EXIT SIGINT

echo ">> Temporary output directory ${TEMP_DIR}"

# Ensure we can execute.
chmod +x ${CODEGEN_PKG}/generate-groups.sh
chmod +x ${CODEGEN_PKG}/generate-internal-groups.sh

${CODEGEN_PKG}/generate-groups.sh all \
    ${CLIENT_PKG} \
    ${APIS_PKG} \
    graphql.gloo.solo.io:v1beta1 \
    --output-base "${TEMP_DIR}" --go-header-file "${SCRIPT_ROOT}/hack/boilerplate.go.txt"
# Copy everything back.
cp -a "${TEMP_DIR}/${ROOT_PKG}/." "${SCRIPT_ROOT}/.."

