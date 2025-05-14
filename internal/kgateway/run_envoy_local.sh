#!/bin/bash

IMG=$(kubectl get deploy gw-dp -o jsonpath='{.spec.template.spec.containers[0].image}')
kubectl get cm gw-dp -o jsonpath='{.data.envoy\.yaml}' | sed 's/address: glood.default/address: 127.0.0.1/' > /tmp/envoy.yaml
CMD=$(kubectl get deploy gw-dp -o jsonpath='{.spec.template.spec.containers[0].command}'|jq -r 'join(" ")'| sed 's@/etc/envoy/envoy.yaml@/tmp/envoy.yaml@')
ARGS=$(kubectl get deploy gw-dp -o jsonpath='{.spec.template.spec.containers[0].args}'|jq -r 'join(" ")'| sed 's@/etc/envoy/envoy.yaml@/tmp/envoy.yaml@')

# if cmd is not empty, use it as entry point
DOCKER_ARGS=""
if [ -n "$CMD" ]; then
    DOCKER_ARGS="--entrypoint=$CMD"
fi


docker run --rm --net=host -v /tmp/envoy.yaml:/tmp/envoy.yaml:ro -e ENVOY_UID=0 -e POD_NAME=foo -e POD_NAMESPACE=default $DOCKER_ARGS $IMG $ARGS
