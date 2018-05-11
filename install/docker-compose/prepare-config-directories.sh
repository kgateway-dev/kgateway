#!/bin/bash

mkdir -p ./{_gloo_config/upstreams,_gloo_config/virtualservices,_gloo_secrets,_gloo_files}
mkdir -p ${HOME}/.glooctl/
cat >${HOME}/.glooctl/config.yaml << EOF
FileOptions:
  ConfigDir: ${PWD}/_gloo_config
  FilesDir: ${PWD}/_gloo_files
  SecretDir: ${PWD}/_gloo_secrets
ConfigStorageOptions:
  SyncFrequency: 1000000000
  Type: file
FileStorageOptions:
  SyncFrequency: 1000000
  Type: file
SecretStorageOptions:
  SyncFrequency: 100000
  Type: file
EOF