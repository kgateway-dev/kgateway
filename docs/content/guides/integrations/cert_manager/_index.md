---
title: "Integrating Gloo and Let's Encrypt with cert-manager"
menuTitle: Cert-manager
description: Secure your ingress traffic using Gloo and cert-manager
weight: 20
---

This document shows how to secure your traffic using Gloo and cert-manager. This guide assumes you already 
have a Kubernetes cluster up and running. Further, it assumes your cluster has a load-balancer such that when 
Gloo is installed, the proxy service is granted an external IP. 

---

### Deploy Gloo

To install gloo, run:
```shell
glooctl install gateway
```

### Setup your DNS

In this example we used the domain name `test-123456789.solo.io`. We'll create an `A` record that maps to the IP address of the 
gateway proxy that we installed with Gloo.  

You can run these commands to update AWS route53 through the AWS command line tool 
(remember to replace *HOSTED_ZONE* and *RECORD* with your values):

```shell
export GLOO_HOST=$(kubectl get svc -l gloo=gateway-proxy -n gloo-system -o 'jsonpath={.items[0].status.loadBalancer.ingress[0].ip}')
RECORD=test-123456789
HOSTED_ZONE=solo.io.
ZONEID=$(aws route53 list-hosted-zones|jq -r '.HostedZones[]|select(.Name == "'"$HOSTED_ZONE"'").Id')
RS='{ "Changes": [{"Action": "UPSERT", "ResourceRecordSet":{"ResourceRecords":[{"Value": "'$GLOO_HOST'"}],"Type": "A","Name": "'$RECORD.$HOSTED_ZONE'","TTL": 300} } ]}'
aws route53 change-resource-record-sets --hosted-zone-id $ZONEID --change-batch "$RS"
```

### Install cert manager

The official installation guide is [here](https://docs.cert-manager.io/en/latest/getting-started/install.html). 
You can install with static manifests or helm. For this example we will use the short version - static manifests:

```shell
kubectl create namespace cert-manager
kubectl label namespace cert-manager certmanager.k8s.io/disable-validation=true
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v0.12.0/cert-manager.yaml
```

### Add a Service

Add a service that will get exposed via gloo. In this document we will use the petclinic spring application. 

```shell
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v0.8.4/example/petclinic/petclinic.yaml
```

### Configure access to AWS

We'll need to allow cert manager access to configure DNS records in AWS. 
See cert manager [docs](https://docs.cert-manager.io/en/latest/tasks/acme/configuring-dns01/route53.html) 
for more details on the access requirements for cert-manager. 

Once you have configured access, we will need to add the access keys as a kubernetes secret, so that cert manager can access them:

```shell
export ACCESS_KEY_ID=...
export SECRET_ACCESS_KEY=...
kubectl create secret generic aws-creds -n cert-manager --from-literal=access_key_id=$ACCESS_KEY_ID --from-literal=secret_access_key=$SECRET_ACCESS_KEY
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
          accessKeyID: $(kubectl -n cert-manager get secret aws-creds -o=jsonpath='{.data.access_key_id}'|base64 --decode)
          secretAccessKeySecretRef:
            name: aws-creds
            key: secret_access_key
EOF
```

Wait until the cluster issuer is in ready state:

```
kubectl get clusterissuer letsencrypt-staging -o jsonpath='{.status.conditions[0].type}{"\n"}'
Ready
```

### Create a certificate for our service

Create the certificate for the gloo ingress:
```shell
cat << EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1alpha2
kind: Certificate
metadata:
  name: test-123456789.solo.io
  namespace: gloo-system
spec:
  secretName: test-123456789.solo.io
  dnsNames:
  - test-123456789.solo.io
  issuerRef:
    name: letsencrypt-staging
    kind: ClusterIssuer
EOF
```

Wait a bit and you will see the secret created:
```shell
kubectl -n gloo-system  get secret
NAME                   TYPE                                  DATA      AGE
test-123456789.solo.io kubernetes.io/tls                     2         3h
...
```

Now just create a virtual host with the same secret ref as the name.

---

### Expose the service securly via Gloo

Configure gloo's default virtual service to route to the function and use the certificates.

```shell
cat <<EOF | kubectl create -f -
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: petclinic-ssl
  namespace: gloo-system
spec:
  virtualHost:
    domains:
      - test-123456789.solo.io
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
      name: test-123456789.solo.io
      namespace: gloo-system
EOF
```

---

### Test!

Now we can open the petclinic application at `https://test-123456789.solo.io/`. 