---
title: "AWS Elastic Load Balancers (ELB)"
description: Use Gloo Edge to complement AWS load balancers
weight: 7
---


Gloo Edge is an application (L7) proxy based on [Envoy](https://www.envoyproxy.io) that can act as both a secure edge router and as a developer-friendly Kubernetes ingress/egress (north-south traffic) gateway. There are many benefits to pairing Gloo Edge with one of AWS Elastic Load Balancers (ELB), including better cross availability zone failover and deeper integration with AWS services like AWS Certificate Manager, AWS CLI & CloudFormation, and Route 53 (DNS). 

AWS provides these three (3) types of Elastic Load Balancers (ELB): 
- Classic Load Balancer (CLB) - this is AWS original (and older) load balancer that provides a mixture of TCP/UDP and HTTP capabilities. It predates some of the Virtual Private Cloud (VPC) infrastructure, so AWS recommends new deployments to utilize the other load balancer types.
- Network Load Balancer (NLB) - this is an optimized L4 TCP/UDP load balancer. It provides extremely high throughput (millions of requests per second) while maintaining low latency. This load balancer also has deep integration with other AWS services like Route 53 (DNS).
- Application Load Balancer (ALB) - this is an L7 (HTTP) only load balancer focused on providing HTTP request routing capabilities.

These load balancers support offloading TLS termination and some degree of cross availability zone failover and support.
More details about AWS cloud load balancers are [here](https://docs.aws.amazon.com/elasticloadbalancing/index.html).


---

## Combining with Gloo Edge

Using Gloo Edge with AWS ELBs is recommended for AWS based deployments. The gateways on Gloo Edge include one or more managed Envoy proxies that can handle both TCP/UDP (L4) and HTTP/gRPC (L7) traffic, and the Gloo Edge proxies can also terminate and originate TLS and HTTPS connections. Gloo Edge's configuration has benefits over AWS ELB, especially for EKS/Kubernetes based services, in that Gloo Edge's configurations are Kubernetes Custom Resources (CRDs) that allow development teams to keep service routing configurations with the application code and run through CI/CD. AWS ELBs have an advantage over Gloo Edge in terms of deep integration with AWS infrastructure, giving the AWS Load Balancers a better integration cross availability zone.

In general, we'd recommend using an **AWS Network Load Balancer** (NLB) with Gloo Edge. Gloo Edge provides more application (L7) capabilities than AWS Application Load Balancer (ALB). Gloo Edge's configuration can be managed and deployed like other Kubernetes assets, which allows application teams to move faster by reducing the number of different teams and infrastructure tiers they have to coordinate with as part of a deployment.

{{% notice warning %}}
**Important notes**
- Gloo Edge does not open any proxy ports till at least one Virtual Service is successfully deployed. AWS ELB Health Checks are automatically created and can report that Gloo Edge is unhealthy until the port is open to connections, i.e., at least one Gloo Edge Virtual Service deployed. If your clients see 503 errors, double check the AWS ELB Health Checks are passing as it can take a couple of minutes for them to detect changes in the Gloo Edge proxy port status.
- An AWS NLB has an idle timeout of 350 seconds that [you cannot change](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/network-load-balancers.html#connection-idle-timeout). This limitation can increase the number of reset TCP connections. Although this limitation is of the load balancer in front of the Gloo Edge proxy, and not of the proxy itself, you can configure Gloo Edge to help prevent TCP resets. To set TCP keep alive for downstream connections to Envoy, use [socket options]({{% versioned_link_path fromRoot="/guides/integrations/aws/socket-options/" %}}).
{{% /notice %}}

---

## The two Kubernetes controllers

To configure your NLB, you will probably use annotations on your Kubernetes `Service`, type LoadBalancer.

**Two** distinct controllers can read these annotations. The _old_ one, in the Kubernetes codebase, and the _new_ one called **AWS Load Balancer Controller**. The latter offers more options, as explained below.

While the _legacy_ [in-tree](https://github.com/kubernetes/kubernetes/blob/master/staging/src/k8s.io/legacy-cloud-providers/aws/aws.go) Cloud Controller will recognize some annotations listed in the Kubernetes [documentation](https://kubernetes.io/docs/concepts/services-networking/service/#aws-nlb-support), it is now recommended to deploy the **AWS Load Balancer Controller**.

The [AWS Load Balancer Controller](https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.2/guide/service/annotations/) ([source code](https://github.com/kubernetes-sigs/aws-load-balancer-controller)) has a [deeper integration](https://aws.amazon.com/blogs/containers/introducing-aws-load-balancer-controller/) with both NLBs and ALBs. Once [installed](https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.2/deploy/installation/), you will be able to leverage advanced annotations such as health checks, access logs, and more. 



### Legacy controller: The in-tree Kubernetes Cloud controller

{{% notice note %}}
Today, it's not clear for how long the community will keep supporting the legacy cloud controller. We strongly recommend you migrate to the [AWS Load Balancer Controller]({{% versioned_link_path fromRoot="/guides/integrations/aws/#new-aws-load-balancer-controller" %}}).
{{% /notice %}}

In an AWS EKS cluster, whenever any Kubernetes Service of type `LoadBalancer` deploys, AWS will, by default, create an AWS Classic Load Balancer paired with that Kubernetes Service. AWS will also automatically create Load Balancer Health Checks against the first port listed in that Service. You can influence how AWS creates a Load Balancer for Kubernetes Services by adding [AWS specific annotations](https://kubernetes.io/docs/concepts/services-networking/service/#aws-nlb-support) to your `LoadBalancer` type Service.

Gloo Edge's managed Envoy proxies install on EKS as a `LoadBalancer` type Service named `gateway-proxy`. Gloo Edge's Helm chart allows the user to specify annotations added to Gloo Edge's `gateway-proxy` Service, including adding AWS ELB annotations that influence the AWS ELB associated with the Gloo Edge proxy service.

The most commonly used AWS annotations used with Gloo Edge are:

* `service.beta.kubernetes.io/aws-load-balancer-type` - Associate an AWS Network Load Balancer with the Service (`nlb`|`nlb-ip`). If this annotation is not present, then AWS associates a Classic ELB with this Service.
* `service.beta.kubernetes.io/aws-load-balancer-ssl-cert` - If specified, AWS ELB's configured listener uses TLS/HTTPS with the provided certificate. Value is a valid certificate ARN from AWS Certificate Manager or AWS IAM, e.g. `arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012`.


### Recommended controller: AWS Load Balancer Controller

While you will find official instructions on their [website](https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.2/deploy/installation/), here is a quick way of getting started.

The first step is to create an AWS IAM account that will create the NLB instances, and that is bound to a Kubernetes service-account:

```bash
export CLUSTER_NAME="my-cluster"
export REGION="eu-central-1"
export AWS_ACCOUNT_ID=XXXXXXXXXX
export IAM_POLICY_NAME=AWSLoadBalancerControllerIAMPolicy
export IAM_SA=aws-load-balancer-controller

# Setup IAM OIDC provider for a cluster to enable IAM roles for pods
eksctl utils associate-iam-oidc-provider \
    --region ${REGION} \
    --cluster ${CLUSTER_NAME} \
    --approve

# Fetch the IAM policy required for our Service-Account
curl -o iam-policy.json https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/v2.2.4/docs/install/iam_policy.json

# Create the IAM policy
aws iam create-policy \
    --policy-name ${IAM_POLICY_NAME} \
    --policy-document file://iam-policy.json

# Create the k8s Service Account
eksctl create iamserviceaccount \
--cluster=${CLUSTER_NAME} \
--namespace=kube-system \
--name=${IAM_SA} \
--attach-policy-arn=arn:aws:iam::${AWS_ACCOUNT_ID}:policy/${IAM_POLICY_NAME} \
--override-existing-serviceaccounts \
--approve \
--region ${REGION}

# Check out the new SA in your cluster for the AWS LB controller
kubectl -n kube-system get sa aws-load-balancer-controller -o yaml
```

The next step is to deploy the **AWS Load Balancer Controller**:

```bash
kubectl apply -k "github.com/aws/eks-charts/stable/aws-load-balancer-controller/crds?ref=master"

helm repo add eks https://aws.github.io/eks-charts
helm repo update
helm install aws-load-balancer-controller eks/aws-load-balancer-controller \
  -n kube-system \
  --set clusterName=${CLUSTER_NAME} \
  --set serviceAccount.create=false \
  --set serviceAccount.name=${IAM_SA}
```

From this point, you can use more annotations as [documented here](https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.2/guide/service/annotations/), on the **AWS Load Balancer Controller** website.

The most important annotations you will need from this point are:

```yaml
gloo:
  gatewayProxies:
    gatewayProxy:
      service:
        ...
        extraAnnotations:
          ## /!\ WARNING /!\
          ## values below will only work with the "AWS Load Balancer Controller". Not with the default k8s in-tree controller
          service.beta.kubernetes.io/aws-load-balancer-type: "external" # https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.2/guide/service/nlb/#configuration
          service.beta.kubernetes.io/aws-load-balancer-scheme: internet-facing # https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.2/guide/service/nlb/
          service.beta.kubernetes.io/aws-load-balancer-nlb-target-type: "instance" # https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.2/guide/service/nlb/#instance-mode_1
          ...
```


## NLB with TLS passthrough

By using an AWS Network Load Balancer (NLB) in front of Gloo Edge, you get an additional benefit of TLS passthrough. That is, HTTPS requests pass through the AWS NLB and terminate TLS at the Gloo Edge proxy for extra security. AWS NLB automatically configures listeners for each Kubernetes Service port, so both HTTP and HTTPS ports get exposed through the AWS NLB.

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

## NLB with TLS offloading

If you want to leverage the AWS NLB integration with managed Certificate service (ACM), then terminating the TLS connection at the NLB makes sense. 
In the example below, you will configure the NLB with a x509 certificate and have it proxying requests to Gloo Edge on the HTTP Gateway:

```yaml
gloo:
  gatewayProxies:
    gatewayProxy:
      service:
        extraAnnotations:
          # LEGACY Kubernetes cloud controller
          # service.beta.kubernetes.io/aws-load-balancer-type: nlb

          # NEW AWS Load Balancer Controller
          service.beta.kubernetes.io/aws-load-balancer-type: "external"
          service.beta.kubernetes.io/aws-load-balancer-scheme: internet-facing
          service.beta.kubernetes.io/aws-load-balancer-nlb-target-type: "instance"
          service.beta.kubernetes.io/aws-load-balancer-ssl-cert: "arn:aws:acm:xxxxxxxxxx" # the ARN of your x509 certificate
          service.beta.kubernetes.io/aws-load-balancer-ssl-ports: "443" # enable TLS on this port
          service.beta.kubernetes.io/aws-load-balancer-backend-protocol: http # will not originate HTTPS connection to the backend
          
        httpPort: 80
        httpsFirst: true # workaround for the NLB health check, as explained later
        httpsPort: 443
        type: LoadBalancer
      # **Method A** for proxying from port 443 to port 8080
      podTemplate: 
        # the HTTPS traffic reaching the NLB will be routed to the HTTP Gateway
        httpsPort: 8080
```

The setup above shows how the HTTPS traffic reaching the NLB is redirected to Gloo Edge over port 8080. Also, the NLB will be configured to listen to both on ports 80 and 443. For the port 443 (HTTPS), a certificate will be exposed by the NLB. 

{{% notice info %}}
**httpsFirst field**: For Kubernetes version 1.14 or later, you change the order of the ports in the Kubernetes service with the `httpsFirst: true` field so that the NLB health checks work. For more information, see the [Kubernetes issue](https://github.com/solo-io/gloo/issues/2571).
{{% /notice %}}

### Disabling the Gloo Edge HTTPS Gateway

If you route all the traffic to the HTTP port of Envoy, then you can disable the HTTPS Gateway (Envoy Listener) in Gloo Edge using `disableHttpsGateway`. The traffic reaching the NLB on port 8443 will be refused since there is no active Envoy listener behind the NLB.

```yaml
gloo:
  gatewayProxies:
    gatewayProxy:
      gatewaySettings:
        # disable the Gloo HTTPS Gateway
        disableHttpsGateway: true
      service:
        # The "Method B" shown below is an alternative to the routing configuration explained in the previous paragraph, called "Method A".
        # **Method B**
        # Set the http port to 443 (instead of 80) since we are terminating tls at the proxy and are
        # not performing a tls passthrough, we need the backing port be http 8080
        httpPort: 443       # HTTP backing port is 8080
        # To prevent conflicts with the above change we are making the https port match its default backing port
        httpsPort: 8443     # HTTPS backing port is 8443
        httpsFirst: true # set the HTTPS port as the first element in the Kubernetes service, otherwise Health checks will fail
```

### HTTPS redirect

Another common requirement is to have the HTTP traffic redirected to HTTPS. You will need to tweak the Gloo _Gateways_, as pictured below:

![HTTPS redirect with NLB doing TLS offloading]({{< versioned_link_path fromRoot="/img/https-redirect-tls-offloading.png" >}})

If you want to know more about the purpose of a `Gateway` _Custom Resource_, check out [Multi-gateway deployment]({{% versioned_link_path fromRoot="/installation/advanced_configuration/multi-gw-deployment/" %}}).

Below is a configuration example:

```yaml
gloo:
  gatewayProxies:
    gatewayProxy:
      service:
        extraAnnotations:
          service.beta.kubernetes.io/aws-load-balancer-type: "external"
          service.beta.kubernetes.io/aws-load-balancer-scheme: internet-facing
          service.beta.kubernetes.io/aws-load-balancer-ssl-cert: "arn:aws:acm:xxxxxxxxxx"
          service.beta.kubernetes.io/aws-load-balancer-ssl-ports: "443"
          service.beta.kubernetes.io/aws-load-balancer-backend-protocol: http
        # both the HTTPS and HTTP port are exposed by the NLB
        httpPort: 80
        httpsFirst: true
        httpsPort: 443
        type: LoadBalancer
        kubeResourceOverride:
          spec:
            ports:
              - name: http
                port: 80
                protocol: TCP
                targetPort: 8090 # NLB listener on port 80 will redirect traffic to the Gateway listening on port 8090
              - name: https
                port: 443
                protocol: TCP
                targetPort: 8080 # regular traffic
      gatewaySettings:
        disableHttpsGateway: true
        # the HTTP Gateway listening on port 8080 will exclude traffic that is meant to the special Gateway, handling the HTTPS redirection
        customHttpGateway:
          virtualServiceExpressions:
            expressions:
              - key: gloo-role
                operator: NotEquals
                values:
                  - redirect-https
```

The following _Gateway_ and _VirtualService_ redirect HTTP traffic on the `gloo.example.com` domain to HTTPS:

```bash
AWS_DNS=$(kubectl --namespace='gloo-system' get service/gateway-proxy --output=jsonpath='{.status.loadBalancer.ingress[0].hostname}')
```

```yaml
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy-redirect
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8090
  httpGateway:
    virtualServiceSelector:
      gloo-role: redirect-https
---
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: http-redirect-ssl-vs
  namespace: gloo-system
  labels:
    gloo-role: redirect-https
spec:
  virtualHost:
    domains:
    - 'gloo.example.com'
    routes:
    - matchers:
      - prefix: /
      redirectAction:
        httpsRedirect: true
        hostRedirect: ${AWS_DNS}
```

## More examples with annotations

Below are examples of how deep you can go with the NLB configuration, thanks to the new **AWS Load Balancer Controller**:

```yaml
gloo:
  gatewayProxies:
    gatewayProxy:
      service:
        extraAnnotations:
          service.beta.kubernetes.io/aws-load-balancer-type: "external" # https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.2/guide/service/nlb/#configuration
          service.beta.kubernetes.io/aws-load-balancer-scheme: internet-facing # https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.2/guide/service/nlb/
          service.beta.kubernetes.io/aws-load-balancer-nlb-target-type: "instance" # https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.2/guide/service/nlb/#instance-mode_1
          service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags: "x-via-lb-type=external"
          service.beta.kubernetes.io/aws-load-balancer-ip-address-type: "ipv4" # service.beta.kubernetes.io/aws-load-balancer-ip-address-type: ipv4

          # LB attributes
          service.beta.kubernetes.io/aws-load-balancer-attributes: "load_balancing.cross_zone.enabled=false"

          # Access logs - will be soon deprecated in favor of "aws-load-balancer-attributes"
          service.beta.kubernetes.io/aws-load-balancer-access-log-enabled: "true"
          service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-name: "nlb-access-logs"
          service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-prefix: "gloo-edge-gw"

          # Target group attributes
          service.beta.kubernetes.io/aws-load-balancer-target-group-attributes: "deregistration_delay.timeout_seconds=15,deregistration_delay.connection_termination.enabled=true"
          #service.beta.kubernetes.io/aws-load-balancer-target-group-attributes: "stickiness.enabled=true,stickiness.type=source_ip,proxy_protocol_v2.enabled=true"

          # Backend
          service.beta.kubernetes.io/aws-load-balancer-ssl-ports: "https"

          # Health checks
          service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold: "2" # 2-20
          service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold: "2" # 2-10
          service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval: "10" # 10 or 30
          service.beta.kubernetes.io/aws-load-balancer-healthcheck-path: "/" # suppose you have a route returning 200 on that path
          service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol: "HTTP"
          service.beta.kubernetes.io/aws-load-balancer-healthcheck-port: "traffic-port"
          service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout: "6" # 6 is the minimum
```
