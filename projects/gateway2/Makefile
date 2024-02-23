TAGGED_VERSION ?= vlatest
ifneq (,$(TAGGED_VERSION))
VERSION := $(shell echo $(TAGGED_VERSION) | cut -c 2-)
endif

LDFLAGS := "-X github.com/solo-io/gloo/pkg/version.Version=$(VERSION)"

tests/conformance/conformance_test.go:
	echo "//go:build conformance" > $@
	cat $(shell go list -json -m sigs.k8s.io/gateway-api | jq -r '.Dir')/conformance/conformance_test.go >> $@
	go fmt $@

CONFORMANCE_ARGS:=-gateway-class=gloo-gateway -supported-features=Gateway,ReferenceGrant,HTTPRoute,HTTPRouteQueryParamMatching,HTTPRouteMethodMatching,HTTPRouteResponseHeaderModification,HTTPRoutePortRedirect,HTTPRouteHostRewrite,HTTPRouteSchemeRedirect,HTTPRoutePathRedirect,HTTPRouteHostRewrite,HTTPRoutePathRewrite,HTTPRouteRequestMirror

# TODO (Law[revert]): need to add CLI tests here, see CI pr.yaml workflow
.PHONY: test
test:
	go test -ldflags=$(LDFLAGS) ./...

.PHONY: test-full
test-full:
	go test -ldflags=$(LDFLAGS) -count=1 ./...

.PHONY: conformance
conformance: tests/conformance/conformance_test.go
	go test -ldflags=$(LDFLAGS) -tags conformance -test.v ./tests/conformance/... -args $(CONFORMANCE_ARGS)

conformance-%: tests/conformance/conformance_test.go
	go test -ldflags=$(LDFLAGS) -tags conformance -test.v ./tests/conformance/... -args $(CONFORMANCE_ARGS) \
	-run-test=$*

# internal target used by controller_suite_test.go
envtest:
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)"

#----------------------------------------------------------------------------------
# Env test
#----------------------------------------------------------------------------------
ENVTEST_K8S_VERSION = 1.23

.PHONY: envtest-path
envtest-path:
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path --arch=amd64

ENVTEST = $(DEPSGOBIN)/setup-envtest

DEPSGOBIN:=$(shell pwd)/.bin
export PATH:=$(DEPSGOBIN):$(PATH)
export GOBIN:=$(DEPSGOBIN)

install-go-tools:
	mkdir -p $(DEPSGOBIN)
	test -s $(ENVTEST) || go install sigs.k8s.io/controller-runtime/tools/setup-envtest@15d792835235
