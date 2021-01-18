.PHONY: docker-local server
all: server docker-local

server:
	@CGO_ENABLED=0 GOOS=linux go build -a --ldflags '-extldflags "-static"' \
		-tags "netgo" -installsuffix netgo \
		-o server

docker-local: server
	docker build -t quay.io/solo-io/passthrough-grpc-service-example .

docker-push:
	docker push quay.io/solo-io/passthrough-grpc-service-example

clean:
	@rm -fr server
