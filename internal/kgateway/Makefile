TAGGED_VERSION ?= vlatest
ifneq (,$(TAGGED_VERSION))
VERSION := $(shell echo $(TAGGED_VERSION) | cut -c 2-)
endif

LDFLAGS := "-X github.com/kgateway-dev/kgateway/pkg/version.Version=$(VERSION)"

# TODO (Law[revert]): need to add CLI tests here, see CI pr.yaml workflow
.PHONY: test
test:
	go test -ldflags=$(LDFLAGS) ./...

.PHONY: test-full
test-full:
	go test -ldflags=$(LDFLAGS) -count=1 ./...
