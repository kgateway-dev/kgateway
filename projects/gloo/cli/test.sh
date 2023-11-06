#!/bin/sh

set -eu

python_version=
if [ -x "$(command -v python)" ]; then
  python_version="$(command -v python)"
  echo "Using $python_version"
fi

if [ ! $python_version ]; then
  if [ -x "$(command -v python3)" ]; then
    python_version="$(command -v python3)"
    echo "Using $python_version"
  fi
fi

if [ ! $python_version ]; then
  if [ -x "$(command -v python2)" ]; then
    python_version="$(command -v python2)"
    echo "Using $python_version"
  fi
fi

if [ ! $python_version ]; then
  echo Python is required to install glooctl
  exit 1
fi

if [ -z "${GLOO_VERSION:-}" ]; then
  GLOO_VERSIONS=$(curl -sH"Accept: application/vnd.github.v3+json" https://api.github.com/repos/solo-io/gloo/releases | $python_version -c "import sys; from distutils.version import StrictVersion, LooseVersion; from json import loads as l; releases = l(sys.stdin.read()); releases = [release['tag_name'] for release in releases];  filtered_releases = list(filter(lambda release_string: len(release_string) > 0 and StrictVersion.version_re.match(release_string[1:]) != None and StrictVersion(release_string[1:]) < StrictVersion('2.0.0') , releases)); filtered_releases.sort(key=LooseVersion, reverse=True); print('\n'.join(filtered_releases))")
else
  GLOO_VERSIONS="${GLOO_VERSION}"
fi

for gloo_version in $GLOO_VERSIONS; do
echo "${gloo_version}"
done
