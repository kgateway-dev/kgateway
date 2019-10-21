DOCKER_REPO ?= soloio
APP_NAME := sleeper
VERSION ?= v0.0.1
LDFLAGS := ""
GCFLAGS := all="-N -l"

docker-push:
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $(APP_NAME)-$(GOOS)-$(GOARCH) main.go
	docker build -t $(DOCKER_REPO)/$(APP_NAME):$(VERSION) .
	docker push $(DOCKER_REPO)/$(APP_NAME):$(VERSION)
