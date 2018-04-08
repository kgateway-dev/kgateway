FROM ubuntu
RUN apt-get update && apt-get install -y ca-certificates
COPY function-discovery-debug /function-discovery
ENTRYPOINT ["/function-discovery"]
