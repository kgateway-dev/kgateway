#!/usr/bin/env bash

CONFIG=$@

for line in $CONFIG; do
  eval "export ${line}"
done

# Define variables.
GH_API="https://api.github.com"
GH_REPO="$GH_API/repos/$owner/$repo"
GH_TAGS="$GH_REPO/releases/tags/$tag"
AUTH="Authorization: token $github_api_token"
WGET_ARGS="--content-disposition --auth-no-challenge --no-cookie"
CURL_ARGS="-LJO#"

if [[ "$tag" == 'LATEST' ]]; then
  GH_TAGS="$GH_REPO/releases/latest"
fi

# Validate token.
curl -o /dev/null -sH "$AUTH" $GH_REPO || { echo "Error: Invalid repo, token or network issue!";  exit 1; }



BODY=$(cat <<EOF
{
  "tag_name": "${tag}",
  "target_commitish": "master",
  "name": "${tag}",
  "body": "${tag} release of ${repo} binaries",
  "draft": false,
  "prerelease": false
}
EOF
)

curl -d "${BODY}" -sH "$AUTH" -XPOST ${GH_REPO}/releases
