---
title: "Cert-manager"
menuTitle: Cert-manager
description: Secure your ingress traffic using Gloo Edge and cert-manager
weight: 20
---

Secure ingress traffic to your host domain by using Gloo Edge and cert-manager to manage your domain's certificates. 

The guide includes examples for the following scenarios and Certificate Authorities (CA):
* Verify domain ownership with the [DNS-01 challenge](#dns-01) with Let's Encrypt CA and cert-manager in an AWS environment.
* Verify domain ownership with the [HTTP-01 challenge](#http-01) with Let's Encrypt CA and cert-manager in an AWS environment.
* Set up [HashiCorp Vault as the CA](#vault-ca) for certificates used by cert-manager and Gloo Edge.

## Before you begin

1. [Create a Kubernetes cluster]({{< versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/">}}).
2. Make sure that your cluster has a load-balancer so that when Gloo Edge is installed, the proxy service gets an external IP address.
3. [Install Gloo Edge]({{< versioned_link_path fromRoot="/installation/gateway/kubernetes/">}}).
4. [Install cert-manager](https://cert-manager.io/docs/installation/), such as with the following example command.
   ```shell
   kubectl create namespace cert-manager
   kubectl apply --validate=false -f https://github.com/cert-manager/cert-manager/releases/download/v1.11.0/cert-manager.yaml
   ```
5. Install any tools for your hosting provider, such as the AWS command line interface (CLI) for AWS route53 hosting.


## Verify your domain with the ACME DNS-01 challenge {#dns-01}

To verify ownership of your domain, you can perform the ACME DNS-01 challenge with cert-manager. You must grant cert-manager access to configure DNS records in your hosting provider. The following example shows how to perform this challenge in an AWS environment with Let's Encrypt as the CA. To use HashiCorp Vault instead, refer to [Use HashiCorp Vault as a Certificate Authority](#vault-ca).

### Step 1: Update your AWS record {#dns-01-aws-record}

Update the AWS route53 through the AWS CLI. This example uses the domain name `test-123456789.solo.io` for the *RECORD* and *HOSTED_ZONE*, which you can replace with your own values. You create an `A` record that maps to the IP address of the gateway proxy that you installed with Gloo Edge.

```shell
export GLOO_HOST=$(kubectl get svc -l gloo=gateway-proxy -n gloo-system -o 'jsonpath={.items[0].status.loadBalancer.ingress[0].ip}')
export RECORD=test-123456789
export HOSTED_ZONE=solo.io.
export ROUTE53_ZONE_ID=$(aws route53 list-hosted-zones|jq -r '.HostedZones[]|select(.Name == "'"$HOSTED_ZONE"'").Id')
export RS='{ "Changes": [{"Action": "UPSERT", "ResourceRecordSet":{"ResourceRecords":[{"Value": "'$GLOO_HOST'"}],"Type": "A","Name": "'$RECORD.$HOSTED_ZONE'","TTL": 300} } ]}'
aws route53 change-resource-record-sets --hosted-zone-id $ROUTE53_ZONE_ID --change-batch "$RS"
```

### Step 2: Configure AWS access for cert-manager {#dns-01-aws-access}

Allow cert-manager access to configure DNS records in AWS. For more details on the access requirements for cert-manager, especially for cross-account cases that are not covered in this guide, see the [cert-manager docs](https://cert-manager.io/docs/configuration/acme/dns01/route53/).

Choose between the following options:
* Development and testing environments: [Use an AWS key pair](#using-an-aws-key-pair).
* Production environments: [Use IAM roles for service accounts (IRSA)](#using-aws-irsa).
### Using an AWS key pair

For development and testing environments, you can use an AWS key pair to grant cert-manager access to configure DNS records in AWS.

1. Create access and secret access keys with [required permissions for cert-manager](https://cert-manager.io/docs/configuration/acme/dns01/route53/).
2. Store the keys in a Kubernetes secret, so that cert-manager can access the keys.
   ```shell
   export ACCESS_KEY_ID=...
   export SECRET_ACCESS_KEY=...
   kubectl create secret generic aws-creds -n cert-manager    --from-literal=access_key_id=$ACCESS_KEY_ID    --from-literal=secret_access_key=$SECRET_ACCESS_KEY
   ```
3. Create a cluster issuer for the Let's Encrypt CA with Route 53 that refers to secret.
   ```shell
   cat << EOF | kubectl apply -f -
   apiVersion: cert-manager.io/v1
   kind: ClusterIssuer
   metadata:
     name: letsencrypt-staging
     namespace: gloo-system
   spec:
     acme:
       server: https://acme-staging-v02.api.letsencrypt.org/directory
       email: user@solo.io
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
### Using AWS IRSA

For production environments, you can use AWS IAM roles for service accounts (IRSA) to grant cert-manager access to configure DNS records in AWS.

1. Make sure that IRSA is enabled in your EKS cluster. For more information, see the [AWS IRSA documentation](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html).
2. Save the following information in environment variables.

   ```shell
   export AWS_ACCOUNT=$(aws sts get-caller-identity --query Account | tr -d '"')
   export EKS_CLUSTER_NAME=my-eks-cluster-name
   export EKS_REGION=us-east-1
   export HOSTED_ZONE=solo.io.
   export ROUTE53_ZONE_ID=$(aws route53 list-hosted-zones|jq -r '.HostedZones[]|select(.Name == "'"$HOSTED_ZONE"'").Id')
   export EKS_HASH=$(aws eks describe-cluster --name ${EKS_CLUSTER_NAME} --query cluster.identity.oidc.issuer | cut -d '/' -f5 | tr -d '"')
   ```

3. Create an AWS IAM policy with the minimum required access.

   ```shell
   cat <<EOF > policy.json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Effect": "Allow",
         "Action": "route53:GetChange",
         "Resource": "arn:aws:route53:::change/*"
       },
       {
         "Effect": "Allow",
         "Action": [
           "route53:ChangeResourceRecordSets",
           "route53:ListResourceRecordSets"
         ],
         "Resource": "arn:aws:route53:::hostedzone/*"
       },
       {
         "Effect": "Allow",
         "Action": "route53:ListHostedZonesByName",
         "Resource": "*"
       }
     ]
   }
   EOF
   
   aws iam create-policy \
       --policy-name AwsCertManagerToRoute53 \
       --policy-document file://policy.json
   ```

4. Attach the policy to an AWS IAM role.

   ```shell
   cat <<EOF > trust-policy.json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Effect": "Allow",
         "Action": "sts:AssumeRoleWithWebIdentity",
         "Principal": {
           "Federated": "arn:aws:iam::${AWS_ACCOUNT}:oidc-provider/oidc.eks.${EKS_REGION}.amazonaws.com/id/${EKS_HASH}"
         },
         "Condition": {
           "StringEquals": {
             "oidc.eks.${EKS_REGION}.amazonaws.com/id/${EKS_HASH}:sub": "system:serviceaccount:cert-manager:cert-manager"
           }
         }
       }
     ]
   }
   EOF
   
   aws iam create-role --role-name EksCertManagerRole --assume-role-policy-document file://trust-policy.json
   aws iam attach-role-policy --policy-arn arn:aws:iam::${AWS_ACCOUNT}:policy/AwsCertManagerToRoute53 --role-name EksCertManagerRole
   
   export IAM_ROLE_ARN=$(aws iam get-role --role-name EksCertManagerRole --query Role.Arn | tr -d '"')
   ```

5. Annotate the `cert-manager` service account to use the AWS IAM role to manage Route 53 records.

   ```bash
   kubectl annotate sa -n cert-manager cert-manager "eks.amazonaws.com/role-arn"="${IAM_ROLE_ARN}"
   ```

6. To enable the cert-manager deployment to read the ServiceAccount token, modify the cert-manager deployment to define new file system permissions with the following command. You can also make these changes by upgrading the Helm chart that you used to deployed cert-manager, which persists the changes across upgrades.

   ```
   kubectl patch deployment -n cert-manager cert-manager --type "json" -p '[{"op":"add","path":"/spec/template/spec/securityContext/fsGroup","value":1001}]
   ```
7. Create a cluster issuer for the Let's Encrypt CA with Route 53.

   ```shell
   kubectl apply -f - <<EOF
   apiVersion: cert-manager.io/v1
   kind: ClusterIssuer
   metadata:
     name: letsencrypt-prod
   spec:
     acme:
       server: https://acme-v02.api.letsencrypt.org/directory
       email: user@solo.io
       privateKeySecretRef:
         name: letsencrypt-prod
       solvers:
       - selector:
           dnsZones:
             - "${HOSTED_ZONE}"
         dns01:
           route53:
             region: ${EKS_REGION}
   EOF
   ```


### Step 3: Configure Gloo Edge resources to verify your domain {#dns-01-aws-edge}

Now that the AWS access is configured, you can configure the Gloo Edge resources to test your verified domain.

1. Make sure that the Let's Encrypt cluster issuer is in a ready state.

   ```
   kubectl get clusterissuer letsencrypt-staging -o jsonpath='{.status.conditions[0].type}{"\n"}'
   Ready
   ```
2. Create the certificate for the Gloo Edge ingress traffic along your host domain, such as `test-123456789.solo.io`.
   ```shell
   cat << EOF | kubectl apply -f -
   apiVersion: cert-manager.io/v1
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
3. Make sure that the Kubernetes secret for the TLS certificate is created.
   ```shell
   kubectl -n gloo-system  get secret
   NAME                   TYPE                                  DATA      AGE
   test-123456789.solo.io kubernetes.io/tls                     2         3h
   ```
4. Configure Gloo Edge's default VirtualService to refer to the TLS secret and to route the pet clinic sample app to the host domain.
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
         routeAction:
           single:
             upstream:
                 name: default-petclinic-80
                 namespace: gloo-system
     sslConfig:
       secretRef:
         name: test-123456789.solo.io
         namespace: gloo-system
   EOF
   ```

5. In your browser, open the host domain, such as `https://test-123456789.solo.io/`. You see the pet clinic sample app!

## Verify your domain with the ACME HTTP-01 Challenge {#http-01}

To verify ownership of your domain, you can perform the ACME HTTP-01 challenge with cert-manager. The HTTP-01 challenge has the ACME server (Let's Encrypt) pass a token to your ACME client (cert-manager). The token is reachable on your domain along a "well known" at `http://<YOUR_DOMAIN>/.well-known/acme-challenge/<TOKEN>`. Unlike the DNS-01 challenge, the HTTP-01 challenge does not require you to change your DNS configuration. As such, you might use the HTTP-01 challenge when you want a simpler and automatable verification method. 

The following example shows how to perform this challenge in an AWS environment with Let's Encrypt as the CA. To use HashiCorp Vault instead, refer to [Use HashiCorp Vault as a Certificate Authority](#vault-ca). A `LoadBalancer` service in the cluster provides the external IP address. [nip.io](https://nip.io/) maps this IP address to a specific domain name via DNS. 

{{% notice note %}}
These steps are specific for Gloo Edge running in gateway mode. When running in ingress mode, cert-manager automatically creates the `Ingress` resources. Therefore, you can skip adding or modifying the VirtualService.
{{% /notice %}}

### Step 1: Create the ClusterIssuer and Certificate resources for the HTTP-01 challenge {#http-01-certs}

1. Create a `ClusterIssuer` with the following details:
   * Uses the `http01` solver for the HTTP-01 challenge.
   * Sets the ingress service type to `ClusterIP`.  By default, cert-manager creates a NodePort service that an Ingress resource routes to. However, because you run Gloo Edge in gateway mode, incoming traffic is routed through a VirtualService instead. Therefore, you do not need a NodePort and can set the service type to ClusterIP.
   * Sets the `dnsName` to be a [nip.io](https://nip.io/) subdomain with the IP address of the externally facing LoadBalancer IP address. The inline command uses `glooctl proxy address` to get the externally facing IP address of the proxy. Then, you append the `nip.io` domain, which results in a domain that looks something like: `34.71.xx.xx.nip.io`.
   ```shell
   cat << EOF | kubectl apply -f -
   apiVersion: cert-manager.io/v1
   kind: ClusterIssuer
   metadata:
     name: letsencrypt-staging-http01
     namespace: gloo-system
   spec:
     acme:
       server: https://acme-staging-v02.api.letsencrypt.org/directory
       email: user@solo.io
       privateKeySecretRef:
         name: letsencrypt-staging-http01
       solvers:
       - http01:
           ingress:
             serviceType: ClusterIP
         selector:
           dnsNames:
           - $(glooctl proxy address | cut -f 1 -d ':').nip.io
   EOF
   ```
2. Create the `Certificate` that uses the `ClusterIssuer`. Behind the scenes, cert-manager creates the relevant `CertificateRequest` and `Order` resources. To satisfy the order, cert-manager spins up a pod and service that present the correct token.

   ```shell
   cat << EOF | kubectl apply -f -
   apiVersion: cert-manager.io/v1
   kind: Certificate
   metadata:
     name: nip-io
     namespace: default
   spec:
     secretName: nip-io-tls
     issuerRef:
       kind: ClusterIssuer
       name: letsencrypt-staging-http01
     commonName: $(glooctl proxy address | cut -f 1 -d ':').nip.io
     dnsNames:
       - $(glooctl proxy address | cut -f 1 -d ':').nip.io
   EOF
   ```

### Step 2: Configure the routing resources {#http-01-routing}

Now that the pod to serve the token is created, you must configure Gloo Edge to route to the pod. You can create a VirtualService for the custom domain that routes requests for the path `/.well-known/acme-challenge/<TOKEN>` to the cert-manager token pod.

1. Check that the token pod and service are running in the `default` namespace.
   ```shell
   kubectl get pod
   kubectl get service
   ```
   Example output:
   ```
   NAME                        READY   STATUS    RESTARTS   AGE
   cm-acme-http-solver-s69mw   1/1     Running   0          1m6s
   NAME                        TYPE           CLUSTER-IP      EXTERNAL-IP      PORT(S)                               AGE
   cm-acme-http-solver-f6mdb   ClusterIP      10.35.254.161   <none>           8089/TCP                              2m5s
   ```
2. With Upstream discovery enabled, Gloo Edge automatically creates an Upstream to the service.
   ```shell
   % glooctl get us default-cm-acme-http-solver-f6mdb-8089
   +----------------------------------------+------------+----------+--------------------------------+
   |                UPSTREAM                |    TYPE    |  STATUS  |            DETAILS             |
   +----------------------------------------+------------+----------+--------------------------------+
   | default-cm-acme-http-solver-f6mdb-8089 | Kubernetes | Accepted | svc name:                      |
   |                                        |            |          | cm-acme-http-solver-f6mdb      |
   |                                        |            |          | svc namespace: default         |
   |                                        |            |          | port:          8089            |
   |                                        |            |          |                                |
   +----------------------------------------+------------+----------+--------------------------------+
   ```
3. To view the `token` value for the `Order`, inspect the `Order`.
   ```shell
   kubectl get orders.acme.cert-manager.io nip-io-556035424-1317610542 -o=jsonpath='{.status.authorizations[0].challenges[?(@.type=="http-01")].token}'
   q5x9q1C4pPg1RtDEiXK9aMAb9ExpepU4Pp14pGKDPXo
   ```
4. Create a VirtualService to route to the cert-manager token pod at the expected well known path. Note that the domain matches the [nip.io](https://nip.io/) domain and routes requests for the path that Let's Encrypt expects, `/.well-known/acme-challenge/<TOKEN>` to the Upstream.

   ```shell
   cat << EOF | kubectl apply -f -
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: letsencrypt
     namespace: gloo-system
   spec:
     virtualHost:
       domains:
       - $(glooctl proxy address | cut -f 1 -d ':').nip.io
       routes:
       - matchers:
         - exact: /.well-known/acme-challenge/q5x9q1C4pPg1RtDEiXK9aMAb9ExpepU4Pp14pGKDPXo
         routeAction:
           single:
             upstream:
               name: default-cm-acme-http-solver-f6mdb-8089
               namespace: gloo-system
   EOF
   ```

### Step 3: Verify that the challenge is complete {#http-01-verify}

Now that the server can reach the cert-manager token pod, the HTTP-01 challenge is complete.

1. Check that the TLS `Certificate` is available.

   ```shell
   % kubectl get certificates.cert-manager.io
   NAME     READY   SECRET       AGE
   nip-io   True    nip-io-tls   10m
   ```
2. Check that you have a sample app, such as pet store.

   ```shell
   kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.11.x/example/petstore/petstore.yaml
   ```
3. Configure the VirtualService to use the newly created TLS secret and to route to the pet store sample app.
   ```shell
   cat << EOF | kubectl apply -f -
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: letsencrypt
     namespace: gloo-system
   spec:
     virtualHost:
       domains:
       - $(glooctl proxy address | cut -f 1 -d ':').nip.io
       routes:
       - matchers:
         - prefix: /
         routeAction:
           single:
             upstream:
               name: default-petstore-8080
               namespace: gloo-system
     sslConfig:
       secretRef:
         name: nip-io-tls
         namespace: default
   EOF
   ```
4. Send a request to the sample app and verify that the response is returned. Note that the `-k` flag means that `curl` does not verify the certificate. The certificate was generated from Let's Encrypt's staging CA, which is not trusted by the system.
   ```shell
   % curl https://$(glooctl proxy address | cut -f 1 -d ':').nip.io/api/pets -k
   [{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
   ```
5. Inspect the certificate that Envoy gets for this route.
   ```shell
   % openssl s_client -connect $(glooctl proxy address | cut -f 1 -d ':').nip.io:443
   ```

   Example output: You get back information about the certificate used for this connection, such as the following.

   ```
   subject=/CN=34.71.xx.xx.nip.io
   issuer=/CN=Fake LE Intermediate X1
   ```

You just confirmed that the service is accessible over the HTTPS port and that Envoy gets the certificate from Let's Encrypt!

## Use HashiCorp Vault as a Certificate Authority {#vault-ca}

Cert-manager supports using HashiCorp Vault as a CA. For Gloo Edge to use the certificates from Vault, you must set up a Vault instance as the CA. Then, set up cert-manager to use that Vault instance as an `Issuer`.

1. [Set up Vault as a CA by using the PKI secrets engine to generate certificates](https://developer.hashicorp.com/vault/docs/secrets/pki).
2. [Create a cert-manager `Issuer` for the Vault CA](https://cert-manager.io/docs/configuration/vault/).
3. If you use Vault to store other, non-TLS secrets along with the TLS certificates, then configure you default Gloo Edge Settings.
   ```shell
   kubectl -n gloo-system edit settings default
   ```
4. Update the Settings as follows:
   * Remove the existing `kubernetesSecretSource`, `vaultSecretSource`, or `directorySecretSource` field.
   * Add the `secretOptions` section with a Kubernetes source and a Vault source specified to enable secrets to be read from both Kubernetes and Vault.
   * Add the `refreshRate` field to watch for changes in Vault secrets and the local filesystem of where Gloo Edge runs.
   {{< highlight yaml "hl_lines=16-28" >}}
   apiVersion: gloo.solo.io/v1
   kind: Settings
   metadata:
     name: default
     namespace: gloo-system
   spec:
     discoveryNamespace: gloo-system
     gateway:
       validation:
         alwaysAccept: true
         proxyValidationServerAddr: gloo:9988
     gloo:
       xdsBindAddr: 0.0.0.0:9977
     kubernetesArtifactSource: {}
     kubernetesConfigSource: {}
     # Delete or comment out the existing \*SecretSource field
     #kubernetesSecretSource: {}
     secretOptions:
       sources:
       - kubernetes: {}
       # Enable secrets to be read from and written to HashiCorp Vault
       - vault:
         # Add the address that your Vault instance is routeable on
         address: http://vault:8200
         accessToken: root
     # Add the refresh rate for polling config backends for changes
     # This setting is used for watching vault secrets and by other resource clients
     refreshRate: 15s
     requestTimeout: 0.5s
   {{< /highlight >}}

For a more in-depth guide to configuring Vault as a secret source, refer to [Storing Gloo Edge secrets in HashiCorp Vault]({{< versioned_link_path fromRoot="/installation/advanced_configuration/vault_secrets">}}).
