#!/usr/bin/env bash

consul agent -dev --client 0.0.0.0 &

vault server -dev -dev-root-token-id=root \
    -log-level=trace \
    -dev-listen-address 0.0.0.0:8200 &

sleep 1

VAULT_ADDR=http://127.0.0.1:8200 vault policy write gloo ./gloo-policy.hcl

LINUX_ARGS=
if [[ "$OSTYPE" == "linux-gnu" ]]; then
    LINUX_ARGS=--network-interface=docker0
fi

nomad agent -dev \
    --bind 0.0.0.0 $LINUX_ARGS \
    --vault-enabled=true \
    --vault-address=http://127.0.0.1:8200 \
    --vault-token=root &

FAIL=0

trap 'kill $(jobs -p)' EXIT

for job in `jobs -p`
do
echo ${job}
    wait ${job} || let "FAIL+=1"
done

echo ${FAIL} failed
