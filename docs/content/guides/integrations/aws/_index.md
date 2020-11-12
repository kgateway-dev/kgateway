---
title: "AWS Elastic Load Balancers (ELB)"
description: Use Gloo Edge to complement AWS load balancers
weight: 7
---

Gloo Edge is an application (L7) proxy based on [Envoy](https://www.envoyproxy.io) that can act as both a secure edge router and as a developer friendly Kubernetes ingress/egress (north-south traffic) gateway. There are many benefits to pairing Gloo Edge with one of AWS Elastic Load Balancers (ELB), including better cross availability zone failover and deeper integration with AWS services like AWS Certificate Manager, AWS CLI & CloudFormation, and Route 53 (DNS). AWS provides three (3) types of load balancers: Classic Load Balancer (ELB or CLB), Network Load Balancer (NLB), and an Application Load Balancer (ALB). Gloo Edge works well with any of these AWS load balancers though our recommendation is to prefer AWS Network Load Balancer as that has the least capabilities overlap and the best value when paired with Gloo Edge. Gloo Edge provides all of the L7 HTTP/S and gRPC routing, security, and web application firewall capabilities (and much more) that either Classic Load Balancer or Application Load Balancer provides. And since Gloo Edge leverages Envoy, Gloo Edge benefits from growing Envoy community, including AWS contributions, and Envoy's extensible filter architecture to provide for customized extensions when needed.

---

## AWS Load Balancers

AWS provides three (3) types of load balancers:

* Classic Load Balancer (ELB or CLB) - this is AWS original (and older) load balancer that provides a mixture of TCP/UDP and HTTP capabilities. It predates some of the Virtual Private Cloud (VPC) infrastructure, so AWS is currently recommending new deployments utilize the other load balancer types.

* Network Load Balancer (NLB) - this is an optimized L4 TCP/UDP load balancer. It provides extremely high throughput (millions of requests per second) while maintaining low latency. This load balancer also has deep integration with other AWS services like Route 53 (DNS).

* Application Load Balancer (ALB) - this is an L7 (HTTP) only load balancer focused on providing HTTP request routing capabilities.

All of these load balancers support offloading TLS termination and some degree of cross availability zone failover and support.

More details about AWS cloud load balancers are [here](https://docs.aws.amazon.com/elasticloadbalancing/index.html).

---

## Combining with Gloo Edge

Using Gloo Edge with AWS ELBs is recommended for AWS based deployments. The gateways on Gloo Edge include one or more managed Envoy proxies that can manage both TCP/UDP (L4) and HTTP/gRPC (L7) traffic, and the Gloo Edge proxies can also terminate and originate TLS and HTTPS connections. Gloo Edge's configuration has benefits over AWS ELB, especially for EKS/Kubernetes based services, in that Gloo Edge's configurations are Kubernetes Custom Resources (CRDs) that allow development teams to keep service routing configurations with the application code and run through CI/CD. AWS ELBs have an advantage over Gloo Edge in terms of deep integration with AWS infrastructure, giving the AWS Load Balancers a better integration cross availability zone.

In general, we'd recommend using an AWS Network Load Balancer (NLB) with Gloo Edge. Gloo Edge provides more application (L7) capabilities than AWS Application Load Balancer (ALB). Gloo Edge's configuration can be managed and deployed like other Kubernetes assets, which allow application teams to move faster by reducing the number of different teams and infrastructure tiers they have to coordinate with as part of a deployment.

---

## How To

In an AWS EKS cluster, whenever any Kubernetes Service of type `LoadBalancer` deploys, AWS will, by default, create an AWS Classic Load Balancer paired with that Kubernetes Service. AWS will also automatically create Load Balancer Health Checks against the first port listed in that Service. You can influence some of how AWS creates a Load Balancer for Kubernetes Services by adding [AWS specific annotations](https://v1-17.docs.kubernetes.io//docs/concepts/cluster-administration/cloud-providers/#aws) to your `LoadBalancer` type Service.

Gloo Edge's managed Envoy proxies install on EKS as a `LoadBalancer` type Service named `gateway-proxy`. Gloo Edge's Helm chart allows the user to specify annotations added to Gloo Edge's `gateway-proxy` Service, including adding AWS ELB annotations that influence the AWS ELB associated with the Gloo Edge proxy service.

The most commonly used AWS annotations used with Gloo Edge are:

* `service.beta.kubernetes.io/aws-load-balancer-type` - the only acceptable value is `nlb` to associate an AWS Network Load Balancer with the Service. If this annotation is not present, then AWS associates a Classic ELB with this Service.
* `service.beta.kubernetes.io/aws-load-balancer-ssl-cert` - If specified, AWS ELB's configured listener uses TLS/HTTPS with the provided certificate. Value is a valid certificate ARN from AWS Certificate Manager or AWS IAM, e.g. `arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012`.

{{% notice warning %}}
Gloo Edge does not open any proxy ports till at least one Virtual Service is successfully deployed. AWS ELB Health Checks are automatically created and can report that Gloo Edge is unhealthy until the port is open to connections, i.e., at least one Gloo Edge Virtual Service deployed. If your clients see 503 errors, double check the AWS ELB Health Checks are passing as it can take a couple of minutes for them to detect changes in the Gloo Edge proxy port status.
{{% /notice %}}

### Gloo Edge Helm install examples

These examples install Gloo Edge onto an existing AWS EKS cluster and associate an AWS Network Load Balancer (NLB) with the Gloo Edge proxy.

{{< tabs >}}

{{< tab name="Gloo Edge Open Source" codelang="shell">}}
kubectl create namespace 'gloo-system'

helm repo add gloo 'https://storage.googleapis.com/solo-public-helm'

helm install gloo gloo/gloo \
  --namespace='gloo-system' \
  --version="${GLOO_VERSION}" \
  --values - <<EOF
gatewayProxies:
  gatewayProxy:
    service:
      extraAnnotations:
        service.beta.kubernetes.io/aws-load-balancer-type: nlb
EOF
{{< /tab >}}

{{< tab name="Gloo Edge Enterprise" codelang="shell" >}}
kubectl create namespace 'gloo-system'

helm repo add glooe 'http://storage.googleapis.com/gloo-ee-helm'

helm install glooe glooe/gloo-ee \
  --namespace='gloo-system' \
  --version="${GLOO_VERSION}" \
  --set="license_key=${GLOOE_LICENSE_KEY}" \
  --values - <<EOF
gloo:
  gatewayProxies:
    gatewayProxy:
      service:
        extraAnnotations:
          service.beta.kubernetes.io/aws-load-balancer-type: nlb
EOF
{{< /tab >}}

{{< /tabs >}}

### Passthrough TLS

By using an AWS Network Load Balancer (NLB) in front of Gloo Edge, you get an additional benefit of TLS passthrough. That is, HTTPS requests passthrough the AWS NLB and terminate TLS at the Gloo Edge proxy for extra security. AWS NLB automatically configures listeners for each Kubernetes Service port, so both HTTP and HTTPS ports get exposed through the AWS NLB.

Note: Gloo Edge only opens proxy ports when a Virtual Service is successfully deployed and using that port. That is, Virtual Services with `sslConfig` opens the HTTPS proxy port. Virtual Services without the `sslConfig` only open the HTTP port. It may take a couple of minutes for AWS NLB health checks to mark the proxy ports as healthy after a first Virtual Service deploys, so be patient.

```shell
# For this example, let's create a self-signed certificate for your DNS. You
# should use a proper public CA cert for production.
openssl req -x509 -nodes \
  -days 365 \
  -newkey rsa:2048 \
  -keyout tls.key \
  -out tls.crt \
  -subj '/CN=gloo.example.com'

# Create a Kubernetes secret with TLS certificate key and cert
kubectl create secret tls gateway-tls \
  --namespace='gloo-system' \
  --key='tls.key' \
  --cert='tls.crt' \

# Create an example Gloo Edge virtual service with a reference to the TLS secret and
# your DNS domain that is mapped to the AWS NLB IP address (replace `gloo.example.com`)
kubectl apply --filename - <<EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  sslConfig:
    secretRef:
      name: gateway-tls
      namespace: gloo-system
  virtualHost:
    domains:
    - 'gloo.example.com'
    routes:
    - matchers:
      - prefix: /
      directResponseAction:
        status: 200
        body: "Hello, world!"
EOF

# Optional. Create a Virtual Service that redirects HTTP to HTTPS

# Hack to allow us to redirect HTTP => HTTPS given that `gloo.example.com` is
# not a valid DNS. This step is not needed if you have a proper DNS correctly
# mapped to the AWS NLB IP address.
AWS_DNS=$(kubectl --namespace='gloo-system' get service/gateway-proxy --output=jsonpath='{.status.loadBalancer.ingress[0].hostname}')

kubectl apply --filename - <<EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default-https-redirect
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - 'gloo.example.com'
    routes:
    - matchers:
      - prefix: /
      redirectAction:
        hostRedirect: ${AWS_DNS}
        httpsRedirect: true
EOF
```

Test your Gloo Edge services deployed on AWS EKS behind an AWS Network Load Balancer

```shell
# Test TLS termination at Gloo Edge proxy. `--connect-to` option is needed if gloo.example.com is not a DNS mapped to AWS NLB
# IP Address as `--connect-to` redirects connection while preserving SNI information
curl --verbose --cacert tls.crt --connect-to "gloo.example.com:443:$(glooctl proxy address --port='https')" https://gloo.example.com

# Test HTTP => HTTPS redirect
curl -verbose --location --insecure --header "Host: gloo.example.com" $(glooctl proxy url --port='http')
```

### TLS Considerations

You need to think carefully around using TLS in terms of:

* Which segments of the request path should be encrypted/protected
* Which components need access to the request and, therefore, which components need to terminate the TLS connection. For example, any component making routing decisions based on HTTP headers, query path or methods needs access to the decrypted request
* What are your certificate management needs for each component terminating TLS? Who generates the TLS certificate, and how frequently should it be rotated?

Most people use Gloo Edge as an L7 proxy/router, and that means Gloo Edge needs access to the request as cleartext. Enabling access to the request means that either the AWS ELB terminates the TLS connection and/or Gloo Edge terminates, so you could end up with one or two certificates you need to manage.

When using an L4 TCP load balancer, like AWS Network Load Balancer, those will passthrough TLS connections so that you only need to terminate the TLS connection once at the Gloo Edge proxy and only manage one TLS certificate. There are good reasons to terminate TLS connections at the AWS ELB, such as their integration with AWS Certificate Manager and AWS IAM, and offloading CPU workload associated with managing a TLS connection from the Gloo Edge proxy.

Spending a few minutes thinking through the deployment architecture of your TLS connections helps eliminate much frustration and debugging later on.
