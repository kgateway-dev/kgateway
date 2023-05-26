#!/bin/bash

################################################################
# This script checks to see if a changelog file has been added
# with the "skipCI" field set to true.
#
# It will set the SKIP_CI_TESTS env variable to true which can be
# used across steps in the same job
#
# It will  write a file called skip-ci.txt with contents of
# either SKIP_CI_TESTS=true or SKIP_CI_TESTS=false - which can be used
# across different jobs in the same workflow
################################################################

set -ex

skipKubeTestsDirective="skipCI-kube-tests:true"
shouldSkipKubeTests=false

skipDocsBuildDirective="skipCI-docs-build:true"
shouldSkipDocsBuild=false

if [[ $(git diff origin/main HEAD --name-only | grep "changelog/" | wc -l) = "1" ]]; then
    echo "exactly one changelog added since main"
    changelogFileName=$(git diff origin/main HEAD --name-only | grep "changelog/")
    echo "changelog file name == $changelogFileName"
    if [[ $(cat $changelogFileName | grep $skipKubeTestsDirective) ]]; then
        shouldSkipKubeTests=true
    fi
    if [[ $(cat $changelogFileName | grep $skipDocsBuildDirective) ]]; then
        shouldSkipDocsBuild=true
    fi
else
    echo "no changelog found (or more than one changelog found) - not skipping CI"
fi

echo "skip-kube-tests=${shouldSkipKubeTests}" >> $GITHUB_OUTPUT
echo "skip-docs-build=${shouldSkipDocsBuild}" >> $GITHUB_OUTPUT