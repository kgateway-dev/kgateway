#!/bin/bash

set -x

make generated-code -B > /dev/null
if [[ $? -ne 0 ]]; then
  echo "Code generation failed"
  exit 1;
fi
if [[ $(git status --porcelain | wc -l) -ne 0 ]]; then
  echo "Error: Generating code produced a non-empty diff"
  echo "Try running 'make generated-code -B' then re-pushing."
  git status --porcelain
  git diff -U3 | cat
  exit 1;
fi
