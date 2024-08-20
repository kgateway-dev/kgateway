#!/bin/bash

set -e

CURRENT_VERSION=$(git describe --tags --always --abbrev=0)
OLD_VERSION=$(echo -n $CURRENT_VERSION | sed -E 's/.*[^0-9]([0-9]+)$/\1/')
NEW_VERSION=$((OLD_VERSION + 1))
NEXT_VERSION=$(echo -n $CURRENT_VERSION | sed -E "s/$OLD_VERSION$/$NEW_VERSION/")
BRANCH_NAME=$(git symbolic-ref -q HEAD | sed 's#^.*/##')
DESCRIPTION=${DESCRIPTION:=""}

CHANGELOG_DIR="changelog/$NEXT_VERSION"
mkdir -p "$CHANGELOG_DIR"

CHANGELOG_FILE="$CHANGELOG_DIR/$BRANCH_NAME.yaml"

if [[ ! -f $CHANGELOG_FILE ]]; then
    echo "Creating $CHANGELOG_FILE"

    cat << EOF > "$CHANGELOG_FILE"
changelog:
  - type: FIX
    issueLink:
    resolvesIssue: false
    description: >-
      "${DESCRIPTION}"
EOF

echo "Wrote to $CHANGELOG_FILE"
fi

# TODO this is a quick hack to avoid args parsing. If this script grows in complexity,
# parsing args will be more appropriate than this check.
if [[ $# > 0 ]] && [[ "$1" == "edit" ]]; then
    echo "Editing..."
    "$EDITOR" "$CHANGELOG_FILE"
fi
