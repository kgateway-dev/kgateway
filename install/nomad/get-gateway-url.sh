#!/usr/bin/env bash

export GATEWAY_IP=$(docker inspect $(docker ps | grep ingress | awk '{print $1}') -f '{{printf "%v" (index (index (index .NetworkSettings.Ports "8080/tcp") 0) "HostIp")}}')
export GATEWAY_PORT=$(docker inspect $(docker ps | grep ingress | awk '{print $1}') -f '{{printf "%v" (index (index (index .NetworkSettings.Ports "8080/tcp") 0) "HostPort")}}')
export GATEWAY_URL=http://${GATEWAY_IP}:${GATEWAY_PORT}