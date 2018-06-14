This document shows how to secure your ingress traffic using gloo and cert-manager. We will deploy everything
to minikube. With minor adjustments can be applied to any Kubernetes cluster.

# Pre-requisites

A DNS that your control and supported by cert-manager.
In this example we used 'test.solo.io' domain that's managed by AWS Route53.

Kubernets cluster (this document was test with minikube and linux, other OSes\clusters should work with proper adjustments).

# Setup

## Setup your DNS
In this example we used the domain name 'test.solo.io'. We've set an A record for this domain to resolve to the result of `minikube ip` so we can test with minikube.

## Deploy Gloo
To install gloo, run:
```
glooctl install kube
```

## Install cert manager

You can do it via Helm, but for this example we will just use static manifests:

```
kubectl apply -f https://raw.githubusercontent.com/jetstack/cert-manager/master/contrib/manifests/cert-manager/with-rbac.yaml
```


## Add a Service
Add a service that will get exposed via gloo. In this document we will use our beloved pet clinic!

```
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/master/example/demo/yamls/pet-clinic.yaml

```



# Secure Access
Lets provide secure access for it!
For that we will use cert manager to create

use glooctl to create a secret with your current aws credentials (create it in the cert-manager namespace so it can find it):
```
glooctl --kube.namespace=cert-manager secret create aws --name us-east-1
```

Create a cluster issuer for let's encrypt with route53.

```
cat << EOF | kubectl apply -f -
apiVersion: certmanager.k8s.io/v1alpha1
kind: ClusterIssuer
metadata:
spec:
  name: letsencrypt-dns-prod
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: yuval@solo.io
    privateKeySecretRef:
      name: letsencrypt-dns-prod
    dns01:
      providers:
      - name: route53
        route53:
          region: us-east-1
          accessKeyID: $(kubectl -n=cert-manager get secret  -ngloo-system us-east-1  -o=jsonpath='{.data.access_key}'|base64 -d)
          secretAccessKeySecretRef:
            name: us-east-1
            key: secret_key
EOF
```

Create the certificate for the gloo ingress:
```
cat << EOF | kubectl apply -f -
apiVersion: certmanager.k8s.io/v1alpha1
kind: Certificate
metadata:
  name: gloo-ingress
  namespace: gloo-system
spec:
  secretName: gloo-ingress-secret
  dnsNames:
  - test.solo.io
  acme:
    config:
    - dns01:
        provider: route53
      domains:
      - test.solo.io
  issuerRef:
    name: letsencrypt-dns-prod
    kind: ClusterIssuer
EOF
```

Thats it! Wait a bit and you will see the secret created:
```
$ kubectl -ngloo-system  get secret 
NAME                  TYPE                                  DATA      AGE
gloo-ingress-secret   kubernetes.io/tls                     2         3h
...
```

Now just create a virtual host with the same secret ref as the name!

# Expose the service securly via Gloo
Configure gloo's default virtual service to route to the function and use the certificates:

```
$ cat <<EOF | glooctl virtualservice create -f -
name: default
routes:
- request_matcher:
    path_prefix: /
  single_destination:
    upstream:
        name: default-petclinic-80
ssl_config:
  secret_ref: gloo-ingress-secret
EOF
```

# Test!

Get gloo's SSL endpoint:
```
HTTPS_GW=https://test.solo.io:$(kubectl -ngloo-system get service ingress -o jsonpath='{.spec.ports[?(@.name=="https")].nodePort}')
```

Visit the page! display the url with:
```
echo $HTTPS_GW
```

Open the URL in your browser and visit the pet clinic!