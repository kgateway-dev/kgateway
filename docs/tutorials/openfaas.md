This document shows how to access your OpenFaaS functions securly via Gloo. We will deploy everything
to minikube. With minor adjustments can be applied to any kubernets cluster.

# Deploy OpenFaas & Gloo
The official OpenFaas install guide is here: https://docs.openfaas.com/deployment/kubernetes/

The TL;DR version for a minikube setup:

```
git clone https://github.com/openfaas/faas-netes
cd faas-netes && \
kubectl apply -f ./namespaces.yml,./yaml
```

To install gloo, run:
```
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/master/install/kube/install.yaml
```

# Deploy Function

Get access to the OpenFaas gateway, and use faas-cli to deploy a function:

```
GATEWAY=$(minikube service -nopenfaas gateway --url)
faas-cli -g  $GATEWAY deploy --image=alexellis/faas-url-ping --name=url-ping
```

With in a minute or so, you will see a function added to the gateway upstream:
```
$ glooctl upstream get openfaas-gateway-8080 -o yaml
functions:
- name: url-ping
  spec:
    body: null
    headers:
      :method: POST
    passthrough_body: true
    path: /function/url-ping
metadata:
  annotations:
    generated_by: kubernetes-upstream-discovery
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"v1","kind":"Service","metadata":{"annotations":{},"labels":{"app":"gateway"},"name":"gateway","namespace":"openfaas"},"spec":{"ports":[{"nodePort":31112,"port":8080,"protocol":"TCP","targetPort":8080}],"selector":{"app":"gateway"},"type":"NodePort"}}
  namespace: gloo-system
  resource_version: "2909"
name: openfaas-gateway-8080
service_info:
  type: REST
spec:
  service_name: gateway
  service_namespace: openfaas
  service_port: 8080
status:
  state: Accepted
type: kubernetes
```

# Secure Access
Lets provide secure access for it!

create a pair of ssl certs. make sure to provide a Common Name when creating the certificate, 
envoy will reject a certificate without one:
```
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.cert -days 365 -nodes
```

Add the certificates as secrets to kubernetes (this makes them available to gloo):
```
glooctl secret create certificate --name gloo-secure -c server.cert -p server.key
```

Configure gloo's default virtual service to route to the function and use the certificates:
```
$ cat <<EOF | glooctl virtualservice create -f -
name: default
routes:
- request_matcher:
    path_exact: /ping
  single_destination:
    function:
        upstream_name: openfaas-gateway-8080
        function_name: url-ping
ssl_config:
  secret_ref: gloo-secure
EOF
```


# Test!

Get gloo's SSL endpoint:
```
HTTPS_GW=$(minikube service -ngloo-system ingress --url --https | tail -1)
```

Invoke the function:
```
$ curl --cacert server.cert -k $HTTPS_GW/ping -d'https://google.com'  -H"content-type: application/text"
Handle this -> https://google.com
https://google.com => 200
```

Your function will respond and say hi!
