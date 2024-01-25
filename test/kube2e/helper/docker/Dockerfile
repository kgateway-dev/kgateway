FROM cjimti/go-echo:latest

RUN apk update
RUN apk add curl

WORKDIR /

ENTRYPOINT ["/tcp-echo"]