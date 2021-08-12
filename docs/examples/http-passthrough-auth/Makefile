.PHONY: docker-local server
all: server docker-local

server:
	@CGO_ENABLED=0 GOOS=linux go build -a -o server

docker-local: server
	docker build -t gcr.io/solo-public/passthrough-http-service-example .

docker-push: docker-local
	docker push gcr.io/solo-public/passthrough-http-service-example

clean:
	@rm -fr server
