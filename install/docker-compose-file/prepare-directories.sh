#!/usr/bin/env bash

echo "Cleaning the 'data' folder"
rm -rf ./data

echo "Copy the source data into the 'data' folder"
cp -r source_data data

echo "Seeding 'data' folder with other empty directories"
mkdir -p ./data/artifact/artifacts/gloo-system
mkdir -p ./data/config/{authconfigs,gateways,graphqlapis,proxies,ratelimitconfigs,routeoptions,routetables,upstreamgroups,upstreams,virtualhostoptions,virtualservices,httpgateways,tcpgateways}/gloo-system
mkdir -p ./data/secret/secrets/{default,gloo-system}
chmod -R 777 ./data

echo "Done"