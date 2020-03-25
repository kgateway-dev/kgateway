---
title: "Integrating Gloo and Let's Encrypt with cert-manager"
menuTitle: Cert-manager
description: Secure your ingress traffic using Gloo and cert-manager
weight: 20
---

This document shows how to secure your ingress traffic using gloo and cert-manager. We will deploy everything to minikube. With minor adjustments can be applied to any Kubernetes cluster.

---

## Pre-requisites

* A DNS that your control and supported by cert-manager. In this example we used 'test.solo.io' domain that's managed by AWS Route53.

* Kubernetes cluster (this document was test with minikube and linux, other OSes\clusters should work with proper adjustments).

---

## Setup

### Setup your DNS

In this example we used the domain name 'test.solo.io'. We've set an A record for this domain to resolve to the result of `minikube ip` so we can test with minikube.

While you can update your AWS DNS settings through the AWS UI, I find performing changes through the command line faster. Update the DNS record through the AWS command line tool (remember to replace *HOSTED_ZONE* and *RECORD* with your values):

```shell
export INGRESS_HOST=$(kubectl get po -l istio=ingressgateway -n istio-system -o 'jsonpath={.items[0].status.hostIP}')
RECORD=test
HOSTED_ZONE=solo.io.
ZONEID=$(aws route53 list-hosted-zones|jq -r '.HostedZones[]|select(.Name == "'"$HOSTED_ZONE"'").Id')
RS='{ "Changes": [{"Action": "UPSERT", "ResourceRecordSet":{"ResourceRecords":[{"Value": "'$INGRESS_HOST'"}],"Type": "A","Name": "'$RECORD.$HOSTED_ZONE'","TTL": 300} } ]}'
aws route53 change-resource-record-sets --hosted-zone-id $ZONEID --change-batch "$RS"
```

### Deploy Gloo

To install gloo, run:
```shell
glooctl install gateway
```

### Install cert manager

The official installation guide is [here](https://docs.cert-manager.io/en/latest/getting-started/install.html). You can install with static manifests or helm. For this example we will use the short version - static manifests:

```shell
kubectl create namespace cert-manager
kubectl label namespace cert-manager certmanager.k8s.io/disable-validation=true
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v0.12.0/cert-manager.yaml
```

### Add a Service

Add a service that will get exposed via gloo. In this document we will use our beloved pet clinic!

```shell
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v0.8.4/example/petclinic/petclinic.yaml
```

---

## Create an issuer

### Configure access

We'll need to allow cert manager access to configure DNS records in AWS. See cert manager [docs](https://docs.cert-manager.io/en/latest/tasks/acme/configuring-dns01/route53.html) for more details on the access requirements for cert-manager. Once you have configured access, we will need to add the access keys as a kubernetes secret, so that cert manager can access them:

```shell
kubectl create secret generic us-east-1 -n cert-manager --from-literal=access_key_id=$ACCESS_KEY_ID --from-literal=secret_access_key=$SECRET_ACCESS_KEY
```

### Create a cluster issuer
Create a cluster issuer for let's encrypt with route53.

```shell
cat << EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1alpha2
kind: ClusterIssuer
metadata:
  name: letsencrypt-staging
  namespace: gloo-system
spec:
  acme:
    server: https://acme-staging-v02.api.letsencrypt.org/directory
    email: yuval@solo.io
    privateKeySecretRef:
      name: letsencrypt-staging
    solvers:
    - dns01:
        route53:
          region: us-east-1
          accessKeyID: $(kubectl -n cert-manager get secret us-west-2 -o=jsonpath='{.data.access_key_id}'|base64 -d)
          secretAccessKeySecretRef:
            name: us-east-1
            key: secret_access_key
EOF
```

Wait until the cluster issuer is in ready state:

```
kubectl get clusterissuer letsencrypt-staging -o jsonpath='{.status.conditions[0].type}{"\n"}'
Ready
```

---

## Create a certificate for our service

Create the certificate for the gloo ingress:
```shell
cat << EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1alpha2
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
    name: letsencrypt-staging
    kind: ClusterIssuer
EOF
```

Thats it! Wait a bit and you will see the secret created:
```shell
kubectl -ngloo-system  get secret
NAME                  TYPE                                  DATA      AGE
gloo-ingress-secret   kubernetes.io/tls                     2         3h
...
```

Now just create a virtual host with the same secret ref as the name!

---

## Expose the service securly via Gloo

Configure gloo's default virtual service to route to the function and use the certificates.

We can create a virtual service and routes in one of two ways:

1. The glooctl command line
1. A Kubernetes CRD

Please choose **one** of these ways outline below to proceed.

### Via the command line:

```shell
glooctl create vs --name default --namespace gloo-system
glooctl edit vs --name default --namespace gloo-system --ssl-secret-name gloo-ingress-secret --ssl-secret-namespace gloo-system
glooctl add route --name default --namespace gloo-system --path-prefix / --dest-name default-petclinic-80 --dest-namespace gloo-system
```

### Via yaml:

```shell
cat <<EOF | kubectl create -f -
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  virtualHost:
    routes:
    - matchers:
       - prefix: /
      route_action:
        single:
          upstream:
              name: default-petclinic-80
              namespace: gloo-system
  ssl_config:
    secret_ref:
      name: gloo-ingress-secret
      namespace: gloo-system
EOF
```

---

## Test!

Get gloo's SSL endpoint:
```shell
HTTPS_GW=https://test.solo.io:$(kubectl -ngloo-system get service gateway-proxy -o jsonpath='{.spec.ports[?(@.name=="https")].nodePort}')
```

Visit the page! display the url with:
```shell
echo $HTTPS_GW
```

Open the URL in your browser and visit the pet clinic!
