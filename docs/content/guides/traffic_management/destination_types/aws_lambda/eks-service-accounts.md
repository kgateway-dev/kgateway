---
title: AWS Lambda with EKS ServiceAccounts
weight: 101
description: Using EKS ServiceAccounts with Gloo Edge for AWS Lambda
---

AWS offers the ability to associate Kubernetes **Service Accounts** with **IAM Roles**.
This [AWS article](https://docs.aws.amazon.com/eks/latest/userguide/specify-service-account-role.html) 
explains the feature in more detail.  
Gloo Edge supports discovering and invoking **AWS Lambdas** using these projected **Service Accounts**.

The following list describes the different resources that are involved in this setup:
* an EKS cluster with an attached IAM **OpenID Provider** (OP) 
* this OP will generate "WebIdentities" which are reflecting Kubernetes **ServiceAccounts** (SA)
* these "WebIdentities" can assume (AWS) IAM **Roles**
* an IAM **Role** is bound to one or more IAM **Policies**
* IAM **Policies** grant access to AWS resources, AWS **Lambdas** in this case

There are many different ways of building these objects, including using the AWS Management Console.


## Configuring an EKS cluster to use an IAM role

### Step 1: Associate an OpenID Provider to your EKS cluster
The first step is to associate an OpenID Provider to your EKS cluster. A full tutorial can be found [in AWS docs](https://docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html).

Once the cluster exists and is configured properly, return here for the rest of the tutorial. The service account 
webhook is available by default in all EKS clusters, even if the workload does not explicitly show up.

### Step 2: Create an IAM Policy

Create a new IAM Policy which has access to the following four actions for this tutorial to function properly:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "VisualEditor0",
            "Effect": "Allow",
            "Action": [
                "lambda:ListFunctions",
                "lambda:InvokeFunction",
                "lambda:GetFunction",
                "lambda:InvokeAsync"
            ],
            "Resource": "*"
        }
    ]
}
```

### Step 3: Create an IAM Role

Create an IAM Role and attach the policy that you created in step 2. 

Then, you use the [AWS CLI](https://docs.aws.amazon.com/IAM/latest/UserGuide/roles-managingrole-editing-cli.html) to modify the role's trust policy to enable the WebIdentities (projected ServiceAccount) to assume that Role. To find the OIDC provider ID to use with your policy, see the [AWS documentation](https://docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html). 
The following JSON payload shows an example of trust relationship:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "ec2.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    },
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::<ACCOUNT ID>:oidc-provider/oidc.eks.<REGION>.amazonaws.com/id/<OIDC-ID>"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "oidc.eks.<REGION>.amazonaws.com/id/<OIDC-ID>:sub": [
            "system:serviceaccount:gloo-system:discovery",
            "system:serviceaccount:gloo-system:gateway-proxy"
          ]
        }
      }
    }
  ]
}
```

### Step 4: take note of the ARNs
After creating this role the following ENV variables need to be set for the remainder of this demo

     export REGION=<region> # The region in which the lambdas are located.
     export AWS_ROLE_ARN=<role-arn> # The Role ARN of the Role created above.
     export SECONDARY_AWS_ROLE_ARN=<secondary-role-arn> # (Optional): A secondary Role ARN with Lambda access.

The Role ARN will be of the form: `arn:aws:iam::<AWS ACCOUNT ID>:role/<ROLE NAME>`
For more info on ARNs see: https://docs.aws.amazon.com/general/latest/gr/aws-arns-and-namespaces.html


## Deploying Gloo Edge

For the purpose of this tutorial we will be installing open source Gloo Edge, but Gloo Edge Enterpise
will work exactly the same, with slightly different helm values as specified below.

{{< tabs >}}
{{< tab name="Open Source" codelang="shell">}}
helm install gloo gloo/gloo \
 --namespace gloo-system --create-namespace --values - <<EOF
settings:
  aws:
    enableServiceAccountCredentials: true
    stsCredentialsRegion: ${REGION}
gateway:
  proxyServiceAccount:
    extraAnnotations:
      eks.amazonaws.com/role-arn: ${AWS_ROLE_ARN}
discovery:
  serviceAccount:
    extraAnnotations:
      eks.amazonaws.com/role-arn: ${AWS_ROLE_ARN}
EOF
{{< /tab >}}
{{< tab name="Enterprise" codelang="shell">}}
helm install gloo glooe/gloo-ee \
 --namespace gloo-system --create-namespace --set-string license_key=YOUR_LICENSE_KEY --values - <<EOF
gloo:
  settings:
    aws:
      enableServiceAccountCredentials: true
      stsCredentialsRegion: ${REGION}
  gateway:
    proxyServiceAccount:
      extraAnnotations:
        eks.amazonaws.com/role-arn: ${AWS_ROLE_ARN}
  discovery:
    serviceAccount:
      extraAnnotations:
        eks.amazonaws.com/role-arn: ${AWS_ROLE_ARN}
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


## Routing to your Lambda(s)

Now that Gloo Edge is running with our credentials set up, you can go ahead and create the Gloo Edge config to enable routing to your AWS Lambdas.

First create an Upstream CR:

```yaml
kubectl apply -f - <<EOF
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  name: lambda
  namespace: gloo-system
spec:
  aws: 
    region: ${REGION}
    roleArn: ${AWS_ROLE_ARN}
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

Once the Upstream has been accepted we can go ahead and create our Virtual Service:
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
If you want to assume this role via the webtoken rather than in a chained form you may set
envoy.reloadable_features.aws_lambda.sts_chaining to 0.


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

## Preparing for Lambda cold starts

When you invoke a new function in AWS Lambda, you might notice significant latency, or a cold start, as Lambda downloads your code and prepares the execution environment. The latency can vary from under 100 ms to more than 1 second.  The chances of a cold start increase if you write the function in a programming language that takes a long time to start up a VM, such as Java. For more information, see the [AWS blog](https://aws.amazon.com/blogs/compute/operating-lambda-performance-optimization-part-1).

Keep in mind cold start latency as you prepare the timeout values of your Virtual Services. If you do not, you might notice `500`-level server error responses. The following `options` example for a VirtualService allows for a total 35-second timeout window (default is 15 seconds), in which up to three requests with 10-second timeouts will be attempted.

```yaml
      options:
        timeout: 35s  # default value is 15s
        retries:
          retryOn: '5xx'
          numRetries: 3
          perTryTimeout: '10s'
```

For more information about controlling timeout and retry settings, see the [API documentation](https://docs.solo.io/gloo-edge/latest/reference/api/envoy/api/v2/route/route.proto.sk/#retrypolicy).