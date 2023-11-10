TAGGED_VERSION ?= vlatest
ifneq (,$(TAGGED_VERSION))
VERSION := $(shell echo $(TAGGED_VERSION) | cut -c 2-)
endif

LDFLAGS := "-X github.com/solo-io/gloo/pkg/version.Version=$(VERSION)"

build:
	CGO_ENABLED=0 GOOS=linux go build -ldflags=$(LDFLAGS) -o gloo-gateway

build-glooctl:
	CGO_ENABLED=0 GOOS=linux go build -ldflags=$(LDFLAGS) -o glooctl ../../projects/gloo/cli/cmd

docker: build
	docker build -t ghcr.io/solo-io/gloo-gateway/glood:$(VERSION) .
	docker pull quay.io/solo-io/envoy-gloo:1.26.4-patch4
	docker tag quay.io/solo-io/envoy-gloo:1.26.4-patch4 ghcr.io/solo-io/gloo-gateway/gloo-proxy:$(VERSION)

push: docker
	docker push ghcr.io/solo-io/gloo-gateway/glood:$(VERSION)
	docker push ghcr.io/solo-io/gloo-gateway/gloo-proxy:$(VERSION)

.PHONY: helm
helm:
	helm package helm/gloo-gateway --app-version $(VERSION) --version $(VERSION)
	helm push gloo-gateway-$(VERSION).tgz oci://ghcr.io/solo-io/helm-charts


crds/gateway-crds.yaml:
	kubectl kustomize "github.com/kubernetes-sigs/gateway-api/config/crd/experimental?ref=adbd9968d4ff7995af21cb9cd0913288e2b4ca27" -o crds/gateway-crds.yaml

kind:
	./kind.sh
	kind load docker-image ghcr.io/solo-io/gloo-gateway/glood:$(VERSION)
	kind load docker-image ghcr.io/solo-io/gloo-gateway/gloo-proxy:$(VERSION)
	helm upgrade -i default ./helm/gloo-gateway/ --set controlPlane.image.tag=$(VERSION) --set gateway.enabled=false --set develop=true
	kubectl delete rs --all

tests/conformance/conformance_test.go:
	echo "//go:build conformance" > $@
	cat $(shell go list -json -m sigs.k8s.io/gateway-api | jq -r '.Dir')/conformance/conformance_test.go >> $@
	go fmt $@

CONFORMANCE_ARGS:=-gateway-class=gloo-gateway -supported-features=Gateway,ReferenceGrant,HTTPRoute,HTTPRouteQueryParamMatching,HTTPRouteMethodMatching,HTTPRouteResponseHeaderModification,HTTPRoutePortRedirect,HTTPRouteHostRewrite,HTTPRouteSchemeRedirect,HTTPRoutePathRedirect,HTTPRouteHostRewrite,HTTPRoutePathRewrite,HTTPRouteRequestMirror,HTTPRouteRequestMultipleMirrors

.PHONY: test
test:
	go test -ldflags=$(LDFLAGS) ./...

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
