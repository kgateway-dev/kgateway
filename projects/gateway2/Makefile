build:
	CGO_ENABLED=0 GOOS=linux go build -o gloo-gateway2

docker: build
	docker build -t docker.io/soloio/gloo-gateway2:latest .

crds/gateway-crds.yaml:
	kubectl kustomize "github.com/kubernetes-sigs/gateway-api/config/crd/experimental?ref=f98e94a5305e3a08cb14f8a03470f8f3bdf6d54c" -o crds/gateway-crds.yaml

kind:
	kubectl get crd gateways.gateway.networking.k8s.io &> /dev/null || kubectl apply -f crds/gateway-crds.yaml
	kind load docker-image docker.io/soloio/gloo-gateway2:latest
	helm upgrade -i gloo-gateway2 ./helm/gloo-gateway2/ --set controlPlane.image.tag=latest --set gateway.enabled=false --set develop=true
	kubectl delete pod --all
	kubectl delete rs --all

conformance:
	[ -d gateway-api ] || git clone https://github.com/kubernetes-sigs/gateway-api
	cd gateway-api && git checkout $(shell go list -json -m sigs.k8s.io/gateway-api | jq -r '.Version') && go test ./conformance/... -test.v -args \
    -gateway-class=solo-gateway \
    -supported-features=Gateway,HTTPRoute

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
