---
title: AWS Lambda + EKS ServiceAccounts
weight: 101
description: Using EKS ServiceAccounts with Gloo Edge for AWS Lambda
---

# How to use EKS ServiceAccounts to authenticate AWS Lambda requests with Gloo Edge

Recently, AWS added the ability to associate Kubernetes ServiceAccounts with IAM roles.
This [blog post](https://aws.amazon.com/blogs/opensource/introducing-fine-grained-iam-roles-service-accounts/) 
explains the feature in more detail.

Gloo Edge Api Gateway now supports discovering, and authenticating AWS Lambdas in kubernetes using 
these projected ServiceAccounts

## Configuring EKS cluster to use IAM ServiceAccount roles

The first step to enabling this IAM ServiceAccount roles with Gloo Edge is creating/configuring an EKS
cluster to use this feature.

A full tutorial can be found [in AWS' docs](https://docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html).
Once the cluster exists and is configured properly, return here for the rest of the tutorial. The service account 
webhook is available by default in all EKS clusters, even if the workload does not explicitly show up.

Note: The aws role needs to be associated with a policy which has access to the following 4
Actions for this tutorial to function properly.

    * lambda:ListFunctions
    * lambda:InvokeFunction
    * lambda:GetFunction
    * lambda:InvokeAsync

After creating this role the following ENV variables need to be set for the remainder of this demo

    * AWS_REGION: The region in which the lambdas are located
    * AWS_ROLE_ARN: The role ARN of the role created above.
    * $SECONDARY_AWS_ROLE_ARN(optional): A secondary role arn with lambda access.

The ARN will be of the form: `arn:aws:iam::123456789012:user/Development/product_1234/*`
For more info on ARNs see: https://docs.aws.amazon.com/general/latest/gr/aws-arns-and-namespaces.html


## Deploying Gloo Edge

As this feature is brand new, it is currently only available on a beta branch of gloo. The following 
are the version requirements for closed source and open source Gloo Edge.

    Closed Source: v1.5.0-beta8
    
    Open Source: v1.5.0-beta22

For the purpose of this tutorial we will be installing open source Gloo Edge, but closed source Gloo Edge 
will work exactly the same, with slightly different helm values specified below.

{{< tabs >}}
{{< tab name="Open Source" codelang="shell">}}
helm install gloo https://storage.googleapis.com/solo-public-helm/charts/gloo-1.5.0-beta21.tgz \
 --namespace gloo-system --create-namespace --values - <<EOF
settings:
  aws:
    enableServiceAccountCredentials: true
gateway:
  proxyServiceAccount:
    extraAnnotations:
      eks.amazonaws.com/role-arn: $AWS_ROLE_ARN
discovery:
  serviceAccount:
    extraAnnotations:
      eks.amazonaws.com/role-arn: $AWS_ROLE_ARN
EOF
{{< /tab >}}
{{< tab name="Enterprise" codelang="shell">}}
helm install gloo https://storage.googleapis.com/gloo-ee-helm/charts/gloo-ee-1.5.0-beta8.tgz \
 --namespace gloo-system --create-namespace --set-string license_key=YOUR_LICENSE_KEY --values - <<EOF
gloo:
  settings:
    aws:
      enableServiceAccountCredentials: true
  gateway:
    proxyServiceAccount:
      extraAnnotations:
        eks.amazonaws.com/role-arn: "arn:aws:iam::410461945957:role/eksctl-sa-iam-test-addon-iamserviceaccount-d-Role1-1VPIXPWL4TQQM"
  discovery:
    serviceAccount:
      extraAnnotations:
        eks.amazonaws.com/role-arn: "arn:aws:iam::410461945957:role/eksctl-sa-iam-test-addon-iamserviceaccount-d-Role1-1VPIXPWL4TQQM"
EOF
{{< /tab >}}
{{< /tabs >}}


Once helm has finished installing, which we can check by running the following, we're ready to move on.
```shell script
kubectl rollout status deployment -n gloo-system gateway-proxy
kubectl rollout status deployment -n gloo-system gloo
kubectl rollout status deployment -n gloo-system gateway
kubectl rollout status deployment -n gloo-system discovery
```


## Routing to our Lambda

Now that Gloo Edge is running with our credentials set up, we can go ahead and create our Gloo Edge config to 
enable routing to our AWS Lambda

First we need to create our Upstream.
```yaml
kubectl apply -f - <<EOF
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  name: lambda
  namespace: gloo-system
spec:
  aws:
    region: $AWS_REGION
EOF
```

Since FDS is enabled, Gloo Edge will go ahead and discover all available lambdas using the ServicaAccount credentials. 
The lambda we will be using for the purposes of this demo will be called `uppercase`, and it is a very simple lambda
which will uppercase any text in the request body.
```shell script
kubectl get us -n gloo-system lambda -oyaml

apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  name: lambda
  namespace: gloo-system
spec:
  aws:
    lambdaFunctions:
    # ...
    - lambdaFunctionName: uppercase
      logicalName: uppercase
      qualifier: $LATEST
    # ...
    region: us-east-1
status:
  reportedBy: gloo
  state: 1
```

Once the Upstream has been accepted we can go ahead and create our Virtual Service
```shell script
kubectl apply -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - prefix: /lambda
      routeAction:
        single:
          destinationSpec:
            aws:
              logicalName: uppercase
          upstream:
            name: lambda
            namespace: gloo-system
EOF
```

Now we can go ahead and try our route! The very first request will take slightly longer, as the STS credential request 
must be performed in band. However, each subsequent request will be much quicker as the credentials will be cached.
```shell script
curl -v $(glooctl proxy url)/lambda --data '"abc"' --request POST -H"content-type: application/json"
Note: Unnecessary use of -X or --request, POST is already inferred.
*   Trying 3.129.77.154...
* TCP_NODELAY set
* Connected to <redacted> port 80 (#0)
> POST /lambda HTTP/1.1
> Host: <redacted>
> User-Agent: curl/7.64.1
> Accept: */*
> content-type: application/json
> Content-Length: 5
>
* upload completely sent off: 5 out of 5 bytes
< HTTP/1.1 200 OK
< date: Wed, 05 Aug 2020 17:59:58 GMT
< content-type: application/json
< content-length: 5
< x-amzn-requestid: e5cc4545-2989-4105-a4b2-49707d654bce
< x-amzn-remapped-content-length: 0
< x-amz-executed-version: 1
< x-amzn-trace-id: root=1-5f2af39e-5b3e38488ffeb5ec541107d4;sampled=0
< x-envoy-upstream-service-time: 53
< server: envoy
<
* Connection #0 to host <redacted> left intact
"ABC"* Closing connection 0
```

We can also optionally override the role ARN used to authenticate our lambda requests, by adding it into our Upstream
like so:
```shell script
kubectl apply -f - << EOF
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  name: lambda
  namespace: gloo-system
spec:
  aws:
    region: us-east-1
    roleArn: $SECONDARY_AWS_ROLE_ARN
EOF
```

Now we can go ahead and try our route again! Everything should just work, notice that the request may take as long as 
the initial request since the credentials for this ARN have not been cached yet.
```shell script
curl -v $(glooctl proxy url)/lambda --data '"abc"' --request POST -H"content-type: application/json"
Note: Unnecessary use of -X or --request, POST is already inferred.
*   Trying 3.129.77.154...
* TCP_NODELAY set
* Connected to <redacted> port 80 (#0)
> POST /lambda HTTP/1.1
> Host: <redacted>
> User-Agent: curl/7.64.1
> Accept: */*
> content-type: application/json
> Content-Length: 5
>
* upload completely sent off: 5 out of 5 bytes
< HTTP/1.1 200 OK
< date: Wed, 05 Aug 2020 17:59:58 GMT
< content-type: application/json
< content-length: 5
< x-amzn-requestid: e5cc4545-2989-4105-a4b2-49707d654bce
< x-amzn-remapped-content-length: 0
< x-amz-executed-version: 1
< x-amzn-trace-id: root=1-5f2af39e-5b3e38488ffeb5ec541107d4;sampled=0
< x-envoy-upstream-service-time: 53
< server: envoy
<
* Connection #0 to host <redacted> left intact
"ABC"* Closing connection 0
```

