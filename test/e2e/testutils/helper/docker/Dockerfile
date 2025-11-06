# This file is used to build soloio/tcp-echo (https://hub.docker.com/r/soloio/tcp-echo)
# which has not received updates since 2019
FROM cjimti/go-echo:latest

RUN apk update
RUN apk add curl

WORKDIR /

ENTRYPOINT ["/tcp-echo"]
