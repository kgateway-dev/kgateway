#!/bin/bash

set -e

CURRENT_VERSION=$(git describe --tags --always --abbrev=0)
OLD_VERSION=$(echo -n $CURRENT_VERSION | sed -E 's/.*[^0-9]([0-9]+)$/\1/')
NEW_VERSION=$((OLD_VERSION + 1))
NEXT_VERSION=$(echo -n $CURRENT_VERSION | sed -E "s/$OLD_VERSION$/$NEW_VERSION/")
BRANCH_NAME=$(git symbolic-ref -q HEAD | sed 's#^.*/##')
DESCRIPTION=${DESCRIPTION:=""}

mkdir -p "changelog/$NEXT_VERSION"

cat <<EOF > "changelog/$NEXT_VERSION/$BRANCH_NAME.yaml"
changelog:
  - type: FIX
    issueLink:
    resolvesIssue: false
    description: >-
      "${DESCRIPTION}"
EOF
echo Created "changelog/$NEXT_VERSION/$BRANCH_NAME.yaml"