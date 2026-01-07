# this is the latest tag that has been pushed
# if changes are made to the service, this tag should be updated before pushing
TAG ?= 0.0.5

.PHONY: docker-local server
all: server docker-local

server:
	CGO_ENABLED=0 go build -a -o _output/server

docker-local:
	docker buildx build --push \
		--platform linux/amd64,linux/arm64 \
		-t gcr.io/solo-test-236622/ext-proc-example-basic-sink:$(TAG) .

docker-push:
	./publish-interactive.sh $(TAG)

run:
	docker run -p 18080:18080 gcr.io/solo-test-236622/ext-proc-example-basic-sink:$(TAG)

clean:
	rm -fr _output/server
