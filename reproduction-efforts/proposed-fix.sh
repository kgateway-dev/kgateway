# install gloo
helm install -n gloo-system gloo gloo/gloo \
  --create-namespace \
  --version v1.14.0-beta6

# create a self-signed tls cert+key
# https://docs.solo.io/gloo-edge/latest/guides/security/tls/server_tls/
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
   -keyout tls.key -out tls.crt -subj "/CN=petstore.example.com"

kubectl create secret tls gateway-tls --key tls.key \
   --cert tls.crt --namespace gloo-system

rm tls.crt tls.key

# create a virtual service using dfp
kubectl apply -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: test-static
  namespace: gloo-system
spec:
  sslConfig:
    secretRef:
      name: gateway-tls
      namespace: gloo-system
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

# enable dfp at the gateway level
kubectl apply -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  labels:
    app: gloo
  name: gateway-proxy-ssl
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8443
  httpGateway:
    options:
      dynamicForwardProxy: {}
  proxyNames:
    - gateway-proxy
  ssl: true
  useProxyProto: false
EOF

# kubectl port-forward -n gloo-system deployments/gateway-proxy 8080:8080
# curl https://localhost:8443/service/52.200.117.68 -k -v
# curl localhost:8080/service/52.200.117.68
# curl localhost:8080/service/httpbin.org