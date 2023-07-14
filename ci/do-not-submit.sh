#!/bin/bash

# do-not-submit.sh takes one or more file paths as arguments, then checks each
# of those files for comments including the DO_NOT_SUBMIT keyword. If the
# keyword is present, the script exits with -1.
#
# This initial implementation only checks *.go files for single-line comments
# (i.e. starting with // rather than /* ... */).
#
# This script is invoked by .github/workflows/do-not-submit.yaml

if [[ "$#" -lt 1 ]]; then
  echo "Usage: $0 filename [filenames ...]"
  exit -1
fi

NEWLINE=$'\n'
OUTPUT=""
DO_NOT_SUBMIT_REGEX="[[:space:]]*//[[:space:]]*DO_NOT_SUBMIT"

# Keeps track of number of *.go files (as opposed to all files provided)
file_count=0
for filename in "$@"; do
  # Only check *.go files for now
  if [[ "$filename" == *".go" || "$filename" == *"go.mod" || "$filename" == *".proto" ]]; then
    ((file_count++))
    line_number=1
    while IFS= read -r line; do
      if [[ "$line" =~ $DO_NOT_SUBMIT_REGEX ]]; then
        OUTPUT="${OUTPUT}$filename:$line_number contains DO_NOT_SUBMIT${NEWLINE}"
      fi
      ((line_number++))
    done <"$filename"
  fi
done

if [[ "$OUTPUT" == "" ]]; then
  echo "$file_count go files checked, none contain DO_NOT_SUBMIT"
  exit 0
else
  echo "$OUTPUT"
  exit -1
fi
