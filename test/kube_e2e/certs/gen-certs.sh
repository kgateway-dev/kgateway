#!/usr/bin/env bash

export PKI_ROOT="/tmp/pki"
export PKI_ORGANIZATION="Acme Inc."
export PKI_ORGANIZATIONAL_UNIT="IT"
export PKI_COUNTRY="US"
export PKI_LOCALITY="Agloe"
export PKI_PROVINCE="New York"

mkdir $PKI_ROOT
easypki create --filename root --ca "test-ingress"
cp /tmp/pki/root/certs/root.crt .
cp root.crt ../containers/testrunner/root.crt
cp /tmp/pki/root/keys/root.key .
rm -rf /tmp/pki