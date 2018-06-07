#!/bin/bash

# thanks https://stackoverflow.com/questions/25809999/press-y-to-continue-any-other-key-to-exit-shell-script
asksure() {
    while read -r -n 1 -s answer; do
      if [[ $answer = [YyNn] ]]; then
        [[ $answer = [Yy] ]] && retval=0
        [[ $answer = [Nn] ]] && retval=1
        break
      fi
    done

    echo # just a final linefeed, optics...

    return $retval
}
overwrite() {
cat >${HOME}/.glooctl/config.yaml << EOF
FileOptions:
  ConfigDir: ${PWD}/_gloo_config
  FilesDir: ${PWD}/_gloo_config/files
  SecretDir: ${PWD}/_gloo_config/secrets
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
}

echo "creating gloo storage directories"
mkdir -p ./_gloo_config/{upstreams,virtualservices,roles,secrets,files}

mkdir -p ${HOME}/.glooctl/

if [ -f ${HOME}/.glooctl/config.yaml ]; then
    echo -n "Do you wish to edit ${HOME}/.glooctl/config.yaml to set defaults to docker-compose storage (Y/N)? "
    ### using it
    if asksure; then
      echo "Modifying ${HOME}/.glooctl/config.yaml"
      overwrite
    fi
fi

