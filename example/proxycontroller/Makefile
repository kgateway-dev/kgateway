ifeq ($(TAGGED_VERSION),)
	TAGGED_VERSION := $(shell git describe --tags --dirty)
	RELEASE := "false"
endif
VERSION ?= $(shell echo $(TAGGED_VERSION) | cut -c 2-)

GO_BUILD_FLAGS := GO111MODULE=on CGO_ENABLED=0 GOARCH=amd64
GCFLAGS := all="-N -l"

OUTPUT_DIR=_output

$(OUTPUT_DIR)/proxycontroller-linux-amd64:
	$(GO_BUILD_FLAGS) GOOS=linux go build -gcflags=$(GCFLAGS) -o $@ proxycontroller.go

.PHONY: proxycontroller
proxycontroller: $(OUTPUT_DIR)/proxycontroller-linux-amd64

$(OUTPUT_DIR)/Dockerfile.proxycontroller: Dockerfile
	cp $< $@

proxycontroller-docker: $(OUTPUT_DIR)/proxycontroller-linux-amd64 $(OUTPUT_DIR)/Dockerfile.proxycontroller
	docker build $(OUTPUT_DIR) -f $(OUTPUT_DIR)/Dockerfile.proxycontroller \
		-t quay.io/solo-io/proxycontroller:$(VERSION)

proxycontroller-docker-push:
	docker push quay.io/solo-io/proxycontroller:$(VERSION)