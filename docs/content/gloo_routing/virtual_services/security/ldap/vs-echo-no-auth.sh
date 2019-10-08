  kubectl apply -f - << EOF
  apiVersion: gateway.solo.io/v1
  kind: VirtualService
  metadata:
    name: echo
    namespace: gloo-system
  spec:
    displayName: echo
    virtualHost:
      domains:
        - '*'
      routes:
        - matcher:
            prefix: /echo
          routeAction:
            single:
              kube:
                ref:
                  name: http-echo
                  namespace: default
                port: 5678
    EOF
