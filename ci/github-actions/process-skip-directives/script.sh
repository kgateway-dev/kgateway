#!/bin/bash

################################################################
# This script checks to see if a changelog file has been added
# with any "skipCI-*" fields set to true.
#
# This script supports the following fields:
#   skipCI-kube-tests
#   skipCI-docs-build
#
# For each field that we support, the script will output the value
# into the $GITHUB_OUTPUT variable, which then can be consumed
# by other steps in our CI pipeline
################################################################

set -ex

skipKubeTestsDirective="skipCI-kube-tests:true"
shouldSkipKubeTests=false

skipDocsBuildDirective="skipCI-docs-build:true"
shouldSkipDocsBuild=false

githubBaseRef=$1

changelog=$(git diff origin/$githubBaseRef HEAD --name-only | grep "changelog/")
if [[ $(echo $changelog | wc -l | tr -d ' ') = "1" ]]; then
    echo "exactly one changelog added since main"
    echo "changelog file name == $changelog"
    if [[ $(cat $changelog | grep $skipKubeTestsDirective) ]]; then
        shouldSkipKubeTests=true
    fi
    if [[ $(cat $changelog | grep $skipDocsBuildDirective) ]]; then
        shouldSkipDocsBuild=true
    fi
else
    echo "no changelog found (or more than one changelog found) - not skipping CI"
fi

echo "skip-kube-tests=${shouldSkipKubeTests}" >> $GITHUB_OUTPUT
echo "skip-docs-build=${shouldSkipDocsBuild}" >> $GITHUB_OUTPUT