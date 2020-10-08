#!/bin/bash -ex

make docker-push | tee docker-logs.txt

sed -n '/^Successfully tagged /p' docker-logs.txt | sed 's/^Successfully tagged //' > docker-containers.txt

xargs -I % docker build ci/extended-docker --build-arg BASE_IMAGE=% -t %-extended < docker-containers.txt
xargs -I % docker push %-extended < docker-containers.txt

rm docker-logs.txt docker-containers.txt