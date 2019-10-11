VERSION := 0.0.3

app-linux-amd64:
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go  build -o $@ main.go

.PHONY: docker-build
docker-build: app-linux-amd64
	docker build -t soloio/session-affinity-app:$(VERSION) .

.PHONY: docker-push
docker-push: docker-build
	docker push soloio/session-affinity-app:$(VERSION)
