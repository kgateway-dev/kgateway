#!/bin/bash -e
CURRENT_MAJOR_VERSION=1         # major semvar version of supported product
CURRENT_MINOR_VERSION=13        # minor semvar version of supported product
SUPPORTED_VERSIONS=4            # how many versions of product are in active support
LEGACY_CODE_FOLDER="_archive"   # intended destination of legacy changelogs

cd ../changelog

for folder in v*.*.*; do
    folder="${folder#?}"
    semver=( ${folder//./ } )
    major="${semver[0]}"
    minor="${semver[1]}"

    # active version, supported version, legacy version
    if   [ $CURRENT_MAJOR_VERSION = $major ] && [ $CURRENT_MINOR_VERSION = $minor ]; then
        continue
    elif [ $CURRENT_MAJOR_VERSION = $major ] && [ $(($minor+$SUPPORTED_VERSIONS+1)) -gt $CURRENT_MINOR_VERSION ]; then
        dst="$major.$minor"
    else
        dst="$LEGACY_CODE_FOLDER/$major.$minor"
    fi

    mkdir -p $dst
    mv "v$folder" $dst
done
