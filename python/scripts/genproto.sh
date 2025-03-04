#!/usr/bin/env bash
set -e

REPO_ROOT="$(git rev-parse --show-toplevel)"

TMP_DIR=$(mktemp -d)

if [ -z "${ENVOY_VERSION}" ]; then
  echo "ENVOY_VERSION is not set, required to vendor protos"
  exit 1
fi

ENVOY_REPO_ROOT="https://raw.githubusercontent.com/envoyproxy/envoy/${ENVOY_VERSION}"
XDS_REPO_ROOT="https://raw.githubusercontent.com/cncf/xds/main"
PROTOC_GEN_VALIDATE_VERSION="v1.0.4"
PROTOC_GEN_VALIDATE_ROOT="https://raw.githubusercontent.com/bufbuild/protoc-gen-validate/${PROTOC_GEN_VALIDATE_VERSION}"
echo "generating proto files in ${TMP_DIR}"

proto_gen_paths=(
  "${TMP_DIR}"/envoy/type/v3/
  "${TMP_DIR}"/envoy/service/ext_proc/v3
  "${TMP_DIR}"/envoy/config/core/v3
  "${TMP_DIR}"/udpa/annotations
  "${TMP_DIR}"/envoy/extensions/filters/http/ext_proc/v3
  "${TMP_DIR}"/envoy/annotations
  "${TMP_DIR}"/xds/core/v3
  "${TMP_DIR}"/xds/annotations/v3
  "${TMP_DIR}"/validate
  "${TMP_DIR}"/extproto
  "${TMP_DIR}"/enterprise/options/ai/
  "${TMP_DIR}"/solo_io/solo_kit/api/v1/
)

do_curl() {
  local from="$1"
  local to="$2"

  echo "Downloading ${from} to ${to}"
  (cd "${to}" && curl -s -f --max-time 5 --retry 5 --retry-delay 0 --retry-max-time 20 -O "${from}")
}

# TODO: consider cleaning the existing protos in the future
# git rm -r "${REPO_ROOT}"/projects/ai-extension/ai_extension/api/
# # TODO: remove the hack for tmp protos once AI protos are generated from source
# git restore --staged "${REPO_ROOT}"/projects/ai-extension/ai_extension/api/tmp
for path in "${proto_gen_paths[@]}"; do
  mkdir -p "${path}"
done

# Envoy protos
do_curl "${ENVOY_REPO_ROOT}"/api/envoy/service/ext_proc/v3/external_processor.proto "${TMP_DIR}"/envoy/service/ext_proc/v3/
do_curl "${ENVOY_REPO_ROOT}"/api/envoy/type/v3/http_status.proto "${TMP_DIR}"/envoy/type/v3/
do_curl "${ENVOY_REPO_ROOT}"/api/envoy/config/core/v3/base.proto "${TMP_DIR}"/envoy/config/core/v3/
do_curl "${ENVOY_REPO_ROOT}"/api/envoy/extensions/filters/http/ext_proc/v3/processing_mode.proto "${TMP_DIR}"/envoy/extensions/filters/http/ext_proc/v3/
do_curl "${ENVOY_REPO_ROOT}"/api/envoy/annotations/deprecation.proto "${TMP_DIR}"/envoy/annotations/
do_curl "${ENVOY_REPO_ROOT}"/api/envoy/config/core/v3/address.proto "${TMP_DIR}"/envoy/config/core/v3/
do_curl "${ENVOY_REPO_ROOT}"/api/envoy/config/core/v3/backoff.proto "${TMP_DIR}"/envoy/config/core/v3/
do_curl "${ENVOY_REPO_ROOT}"/api/envoy/config/core/v3/http_uri.proto "${TMP_DIR}"/envoy/config/core/v3/
do_curl "${ENVOY_REPO_ROOT}"/api/envoy/type/v3/percent.proto "${TMP_DIR}"/envoy/type/v3/
do_curl "${ENVOY_REPO_ROOT}"/api/envoy/type/v3/semantic_version.proto "${TMP_DIR}"/envoy/type/v3/
do_curl "${ENVOY_REPO_ROOT}"/api/envoy/config/core/v3/extension.proto "${TMP_DIR}"/envoy/config/core/v3/
do_curl "${ENVOY_REPO_ROOT}"/api/envoy/config/core/v3/socket_option.proto "${TMP_DIR}"/envoy/config/core/v3/

# xDS protos
do_curl "${XDS_REPO_ROOT}"/xds/annotations/v3/status.proto "${TMP_DIR}"/xds/annotations/v3/
do_curl "${XDS_REPO_ROOT}"/xds/core/v3/context_params.proto "${TMP_DIR}"/xds/core/v3/
do_curl "${XDS_REPO_ROOT}"/udpa/annotations/migrate.proto "${TMP_DIR}"/udpa/annotations/
do_curl "${XDS_REPO_ROOT}"/udpa/annotations/security.proto "${TMP_DIR}"/udpa/annotations/
do_curl "${XDS_REPO_ROOT}"/udpa/annotations/sensitive.proto "${TMP_DIR}"/udpa/annotations/
do_curl "${XDS_REPO_ROOT}"/udpa/annotations/status.proto "${TMP_DIR}"/udpa/annotations/
do_curl "${XDS_REPO_ROOT}"/udpa/annotations/versioning.proto "${TMP_DIR}"/udpa/annotations/

# Validate proto
do_curl "${PROTOC_GEN_VALIDATE_ROOT}"/validate/validate.proto "${TMP_DIR}"/validate/

# Solo protos
cp "${REPO_ROOT}"/vendor_any/github.com/solo-io/protoc-gen-ext/extproto/ext.proto "${TMP_DIR}"/extproto/
cp "${REPO_ROOT}"/vendor_any/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/ai/ai.proto "${TMP_DIR}"/enterprise/options/ai/
cp "${REPO_ROOT}"/vendor_any/github.com/solo-io/solo-kit/api/v1/ref.proto "${TMP_DIR}"/solo_io/solo_kit/api/v1/
sed -i 's#github.com/solo-io/solo-kit/#solo_io/solo_kit/#g' "${TMP_DIR}"/enterprise/options/ai/ai.proto

echo "generating pb files"

#shellcheck disable=SC2046
python3 -m grpc_tools.protoc --proto_path="${TMP_DIR}" --python_out="${REPO_ROOT}"/projects/ai-extension/ai_extension/api/ --pyi_out="${REPO_ROOT}"/projects/ai-extension/ai_extension/api/ --grpc_python_out="${REPO_ROOT}"/projects/ai-extension/ai_extension/api/ $(find "${TMP_DIR}" -type f -name '*.proto'| tr '\n' ' ')

rm -rf "${TMP_DIR}"