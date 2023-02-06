helm install -n gloo-system gloo gloo/gloo \
  --create-namespace \
  --version v1.14.0-beta6

kubectl apply -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  labels:
    app: gloo
  name: gateway-proxy
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  httpGateway:
    options:
      dynamicForwardProxy: {}
  proxyNames:
    - gateway-proxy
  ssl: false
  useProxyProto: false
EOF

kubectl apply -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: test-static
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - prefix: /service
      options:
        regexRewrite:
          pattern:
            regex: ^/service/.*
          substitution: /get
        stagedTransformations:
          early:
            requestTransforms:
            - requestTransformation:
                transformationTemplate:
                  extractors:
                    destination:
                      header: :path
                      regex: /service/(.*)
                      subgroup: 1
                  headers:
                    x-destination:
                      text: '{{ destination }}'
      routeAction:
        dynamicForwardProxy:
          autoHostRewriteHeader: x-destination
EOF

# kubectl port-forward -n gloo-system deployments/gateway-proxy 8080:8080
# curl localhost:8080/service/52.200.117.68
# curl localhost:8080/service/httpbin.org