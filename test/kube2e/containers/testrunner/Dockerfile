FROM ubuntu:18.04

RUN apt update && apt install -y curl
COPY --from=lachlanevenson/k8s-kubectl:v1.10.3 /usr/local/bin/kubectl /usr/local/bin/kubectl

# Python
RUN apt-get install -y python; apt clean

COPY root.crt /

CMD ["/bin/sh", "-c", "echo 'STARTING SLEEP! Access me.' && /bin/sleep 36000"]