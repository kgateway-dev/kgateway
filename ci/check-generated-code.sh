#!/bin/bash

set -ex

protoc --version

if [ ! -f .gitignore ]; then
  echo "_output" > .gitignore
fi

make install-go-tools

set +e

make generated-code -B > /dev/null
if [[ $? -ne 0 ]]; then
  echo "Code generation failed"
  exit 1;
fi
if [[ $(git status --porcelain | wc -l) -ne 0 ]]; then
  echo "Error: Generating code produced a non-empty diff"
  echo "Try running 'make install-go-tools generated-code -B' then re-pushing."
  git status --porcelain
  git diff | cat
  exit 1;
fi
