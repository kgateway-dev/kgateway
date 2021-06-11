VERSION=0.0.1

.PHONY: docker-local server
all: server-linux-amd64 docker-local docker-push clean

server-linux-amd64:
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go  build -o $@ pkg/server.go

docker-local: server-linux-amd64
	docker build -t quay.io/solo-io/world-cities-soap-service:$(VERSION) .

docker-push:
	docker push quay.io/solo-io/world-cities-soap-service:$(VERSION)

clean:
	@rm -fr server-linux-amd64
