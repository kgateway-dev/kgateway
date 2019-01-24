#!/bin/bash

set -ex

protoc --version

git init
git add .
git commit -m "set up dummy repo for diffing" -q

git clone https://github.com/solo-io/solo-kit /workspace/gopath/src/github.com/solo-io/solo-kit

go get github.com/gogo/protobuf/protoc-gen-gogo
go get github.com/gogo/protobuf/gogoproto
go get golang.org/x/tools/cmd/goimports

PATH=/workspace/gopath/bin:$PATH

make generated-code -B
if [[ $? -ne 0 ]]; then
  echo "Code generation failed"
  exit 1;
fi
if [[ $(git status --porcelain | wc -l) -ne 0 ]]; then
  echo "Generating code produced a non-empty diff"
  git status --porcelain
  git diff | cat
  exit 1;
fi