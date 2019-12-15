#!/bin/bash

set -e

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
ROOT_DIR="${SCRIPT_ROOT}"

#cp -r ${ROOT_DIR}/vendor/github.com ${ROOT_DIR}
#rm -rf ${ROOT_DIR}/vendor/

for file in $(find ${ROOT_DIR} -type f | grep "pb.hash.go")
do
    sed -e "s|interface{}(m.GetStatus())|interface{}(\&m.Status)|g" $file > $file.new
    mv -- "$file.new" "$file"

    sed -e "s|interface{}(m.GetMetadata())|interface{}(\&m.Metadata)|g" $file > $file.new
    mv -- "$file.new" "$file"
done