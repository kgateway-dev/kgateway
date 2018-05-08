This document shows how to access your Fission functions securly via Gloo. We will deploy everything
to minikube. With minor adjustments can be applied to any kubernets cluster.

# Deploy Fission & Gloo
The official Fission install guide is here: https://docs.fission.io/0.6.0/installation/installation/

Setting up kubernetes: https://docs.fission.io/0.6.0/installation/kubernetessetup/


To install gloo, run:
```
glooctl install kube 
```

# Deploy Function

Let's deploy an example function: this can also be found on the install guide, here: https://docs.fission.io/0.6.0/installation/installation/#run-an-example

```
fission env create --name nodejs --image fission/node-env
curl -LO https://raw.githubusercontent.com/fission/fission/master/examples/nodejs/hello.js
fission function create --name hello --env nodejs --code hello.js
fission function test --name hello
```

With in a minute or so, you will see a function added to the gateway upstream:
```
$ glooctl upstream get fission-router-80
+-------------------+------------+----------+----------+
|       NAME        |    TYPE    |  STATUS  | FUNCTION |
+-------------------+------------+----------+----------+
| fission-router-80 | kubernetes | Accepted | hello    |
+-------------------+------------+----------+----------+

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

Configure gloo's default virtual host to route to the function and use the certificates:
```
$ cat <<EOF | glooctl virtualservice create -f -
name: default
routes:
- request_matcher:
    path_exact: /hello
  single_destination:
    function:
        upstream_name: fission-router-80
        function_name: hello
ssl_config:
  secret_ref: gloo-secure
EOF
```


# Test!

Get gloo's SSL endpoint:
```
HTTPS_GW=https://$(minikube ip):$(kubectl get svc ingress -n gloo-system -o 'jsonpath={.spec.ports[?(@.name=="https")].nodePort}')
```

Invoke the function:
```
$ curl --cacert server.cert -k $HTTPS_GW/hello
Hello, world!
```

Your function will respond and say hi!
