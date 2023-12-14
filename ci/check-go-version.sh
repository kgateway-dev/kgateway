#!/bin/bash

# Make sure the minor Go version that we are running matches the version specified in go.mod
goVersion=$(go version | # looks something like "go version go1.20.1 darwin/amd64"
  awk '{print $3}' |     # get the 3rd word -> "go1.20.1"
  sed "s/go//" )         # remove the "go" part -> "1.20.1"
goModVersion=$(grep -m 1 go go.mod | cut -d' ' -f2)

if [[ "$goVersion" == "$goModVersion"* ]]; then
    echo "Using Go version $goVersion"
elif [[ "$goModVersion" == "1.20"* ]] && [[ $goVersion == "1.21"* ]]; then
  # We have upgraded the cloudbuilder environment to use go 1.21.5 to fix a security vulnerability.
  # We build fine with 1.21.5, but the go.mod file still says 1.20 , so we'll add this logic to the support
  # branch to allow the build to continue.
  echo "Using Go version $goVersion with go.mod version $goModVersion for security vulnerability fix."
else
    echo "Your Go version ($goVersion) does not match the version from go.mod ($goModVersion)".
    echo "Please update your Go version to $goModVersion and re-run."
    exit 1;
fi
