FROM bitnami/kubectl:1.28.4 as kubectl

FROM alpine:3.17.6

RUN apk upgrade --update-cache

COPY --from=kubectl /opt/bitnami/kubectl/bin/kubectl /usr/local/bin/
