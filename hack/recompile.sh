#!/usr/bin/env bash

# TODO(ilackarms): refactor this out into setup-new-minishift, apply.sh, and rebuild.sh

set -ex

BASEDIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null && pwd )"

PROJECT=$1
export VERSION=$2
if [ "$#" -ne 2 ]; then
    echo "invalid, to run: ./hack/openshift/recompile.sh PROJECT VERSION"
    exit 1
fi

VM=${VM:-kube}

# won't work for ui...
# need to modify ui make target
make -C ${BASEDIR}/.. $PROJECT-docker
docker save soloio/$PROJECT:$VERSION | ( eval $(mini${VM} docker-env) && docker load)
