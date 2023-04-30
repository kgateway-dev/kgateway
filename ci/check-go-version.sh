#!/bin/bash

# Make sure the minor Go version that we are running matches the version specified in go.mod
goVersion=$(go version | # looks something like "go version go1.20.1 darwin/amd64"
  awk '{print $3}' |     # get the 3rd word -> "go1.20.1"
  sed "s/go//" )         # remove the "go" part -> "1.20.1"
goModVersion=$(grep -m 1 go go.mod | cut -d' ' -f2)

if [[ "$goVersion" == "$goModVersion"* ]]; then
    echo "Using Go version $goVersion"
else
    echo "Your Go version ($goVersion) does not match the version from go.mod ($goModVersion)".
    echo "Please update your Go version to $goModVersion and re-run."
    exit 1;
fi
