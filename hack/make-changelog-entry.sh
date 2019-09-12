#!/usr/bin/env bash

set -ex

REPO=solo-io/gloo

NAME=$1
TYPE=$2
DESCRIPTION=$3
ISSUE=$4

if [[ "$#" -lt 3 ]]; then
    echo "Create a new changelog entry"
    echo "Usage:"
    echo "$(basename "$0") FILENAME NEW_FEATURE|FIX|NON_USER_FACING DESCRIPTION [ISSUE]"
    exit 1
fi

get_latest_release() {
  curl --silent "https://api.github.com/repos/$REPO/releases/latest" | # Get latest release from GitHub api
    grep '"tag_name":' |                                               # Get tag line
    sed -E 's/.*"([^"]+)".*/\1/'                                       # Pluck JSON value
}

write_changelog_issue() {
    cat > changelog/$1/$NAME.yaml <<EOF
changelog:
  - type: $TYPE
    description: ${DESCRIPTION}
    issueLink: https://github.com/${REPO}/issues/${ISSUE}
EOF
}

write_changelog_no_issue() {
    VERSION=$1
    cat > changelog/${VERSION}/$NAME.yaml <<EOF
changelog:
  - type: $TYPE
    description: ${DESCRIPTION}
EOF
}

increment_semver=$(dirname "$0")/increment_semver.sh

VERSION=$(get_latest_release)
VERSION=$($increment_semver -p ${VERSION})

mkdir -p changelog/${VERSION}
if [[ $TYPE == "NON_USER_FACING" ]]; then
    write_changelog_no_issue ${VERSION}
else
    write_changelog_issue ${VERSION}
fi

echo done

