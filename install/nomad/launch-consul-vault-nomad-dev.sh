#!/usr/bin/env bash

consul agent -dev --client 0.0.0.0 &

vault server -dev -dev-root-token-id=root \
    -dev-listen-address 0.0.0.0:8200 &

sleep 1

VAULT_ADDR=http://127.0.0.1:8200 vault policy write gloo ./gloo-policy.hcl

nomad agent -dev \
    --vault-enabled=true \
    --vault-address=http://127.0.0.1:8200 \
    --vault-token=root \
    -bind 172.17.0.1 \
    -network-interface docker0 &

FAIL=0

trap 'kill $(jobs -p)' EXIT

for job in `jobs -p`
do
echo ${job}
    wait ${job} || let "FAIL+=1"
done

echo ${FAIL} failed
