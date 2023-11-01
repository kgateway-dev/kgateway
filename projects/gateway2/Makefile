build:
	CGO_ENABLED=0 GOOS=linux go build -o gloo-gateway2

docker: build
	docker build -t docker.io/soloio/gloo-gateway2:latest .

crds/gateway-crds.yaml:
	kubectl kustomize "github.com/kubernetes-sigs/gateway-api/config/crd/experimental?ref=adbd9968d4ff7995af21cb9cd0913288e2b4ca27" -o crds/gateway-crds.yaml

kind:
	./kind.sh
	kind load docker-image docker.io/soloio/gloo-gateway2:latest
	helm upgrade -i gloo-gateway2 ./helm/gloo-gateway2/ --set controlPlane.image.tag=latest --set gateway.enabled=false --set develop=true
	kubectl delete rs --all

tests/conformance/conformance_test.go:
	echo "//go:build conformance" >  $@
	cat $(shell go list -json -m sigs.k8s.io/gateway-api | jq -r '.Dir')/conformance/conformance_test.go >> $@
	go fmt $@

.PHONY: conformance
conformance: tests/conformance/conformance_test.go
	go test -test.v ./tests/conformance/... -args \
    -gateway-class=solo-gateway \
    -supported-features=Gateway,HTTPRoute

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
