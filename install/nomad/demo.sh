#!/usr/bin/env bash

echo "Launching Consul-Vault-Nomad for Demo (see $(dirname "$0")/.hashi-logs for logs)"
($(dirname "$0")/launch-consul-vault-nomad-dev.sh > .hashi-logs 2>&1) &

sleep 10

VARFILE=""
if [[ "$OSTYPE" == "linux-gnu" ]]; then
    VARFILE=variables/variables-linux.yaml
elif [[ "$OSTYPE" == "darwin"* ]]; then
    VARFILE=variables/variables-mac.yaml
fi

echo "Deploying Gloo"
levant deploy \
    -var-file $VARFILE \
    jobs/gloo.nomad

echo "Deploying Petstore"

levant deploy \
    -var-file $VARFILE \
    jobs/petstore.nomad

FAIL=0

echo "Adding route to Petstore"
glooctl add route \
    --path-prefix / \
    --dest-name petstore \
    --prefix-rewrite /api/pets \
    --use-consul

sleep 3

echo "cURL the gateway"
curl localhost:8080/

trap 'kill $(jobs -p)' EXIT

echo "Ctrl+C to exit."

for job in `jobs -p`
do
echo ${job}
    wait ${job} || let "FAIL+=1"
done

echo ${FAIL} background processes failed
