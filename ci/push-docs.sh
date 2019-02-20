#!/usr/bin/env bash

# Requires $tag shell variable and $GITHUB_TOKEN environment variable

set -e
xargs=$(which gxargs || which xargs)

# Validate settings.
[ "$TRACE" ] && set -x

CONFIG=$@

for line in $CONFIG; do
  eval "export ${line}"
done

github_token_no_spaces=$(echo $GITHUB_TOKEN | tr -d '[:space:]')
branch="docs-gloo-$tag"

set +x
echo "Cloning gloo-docs repo"
git clone https://soloio-bot:$github_token_no_spaces@github.com/solo-io/gloo-docs.git
[ "$TRACE" ] && set -x

git config --global user.name "soloio-bot"
(cd gloo-docs && git checkout -b $branch)

# Gloo
if [ -d "gloo-docs/docs/v1/github.com/solo-io/gloo" ]; then
	rm -r gloo-docs/docs/v1/github.com/solo-io/gloo
fi
cp -r docs/v1/github.com/solo-io/gloo gloo-docs/docs/v1/github.com/solo-io/gloo

# Solo Kit
if [ -d "gloo-docs/docs/v1/github.com/solo-io/solo-kit" ]; then
	rm -r gloo-docs/docs/v1/github.com/solo-io/solo-kit
fi
cp -r docs/v1/github.com/solo-io/solo-kit gloo-docs/docs/v1/github.com/solo-io/solo-kit

# Gogoproto
if [ -d "gloo-docs/docs/v1/gogoproto" ]; then
	rm -r gloo-docs/docs/v1/gogoproto
fi
cp -r docs/v1/gogoproto gloo-docs/docs/v1/gogoproto

# Google
if [ -d "gloo-docs/docs/v1/google" ]; then
	rm -r gloo-docs/docs/v1/google
fi
cp -r docs/v1/google gloo-docs/docs/v1/google

(cd gloo-docs && git add .)

if [[ $( (cd gloo-docs && git status --porcelain) | wc -l) -eq 0 ]]; then
  echo "No changes to gloo docs, exiting."
  rm -rf gloo-docs
  exit 0;
fi

(cd gloo-docs && git commit -m "Add docs for tag $tag")
(cd gloo-docs && git push --set-upstream origin $branch)

curl -v -H "Authorization: token $github_token_no_spaces" -H "Content-Type:application/json" -X POST https://api.github.com/repos/solo-io/gloo-docs/pulls -d \
'{"title":"Update docs for gloo '"$tag"'", "body": "Update docs for gloo '"$tag"'", "head": "'"$branch"'", "base": "master"}'

rm -rf gloo-docs
