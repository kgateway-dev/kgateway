build:
	CGO_ENABLED=0 GOOS=linux go build -o gloo-gateway2

docker: build
	docker build -t docker.io/soloio/gloo-gateway2:latest .

kind:
	kubectl get crd gateways.gateway.networking.k8s.io &> /dev/null || \
      { kubectl kustomize "github.com/kubernetes-sigs/gateway-api/config/crd/experimental?ref=f98e94a5305e3a08cb14f8a03470f8f3bdf6d54c" | kubectl apply -f -; }
	kind load docker-image docker.io/soloio/gloo-gateway2:latest
	helm upgrade -i gloo-gateway2 ./helm/gloo-gateway2/ --set controlPlane.image.tag=latest --set gateway.enabled=false
	kubectl delete pod --all

conformance:
	[ -d gateway-api ] || git clone https://github.com/kubernetes-sigs/gateway-api
	cd gateway-api && git checkout $(shell go list -json -m sigs.k8s.io/gateway-api | jq -r '.Version') && go test ./conformance/... -test.v -args \
    -gateway-class=solo-gateway \
    -supported-features=Gateway,HTTPRoute
