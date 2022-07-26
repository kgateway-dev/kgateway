---
title: AWS Lambda
weight: 100
description: Routing to AWS Lambda as an Upstream
---

Route traffic requests directly to an [Amazon Web Services (AWS) Lambda function](https://aws.amazon.com/lambda/resources/).

## About

Gloo Edge enables you to route traffic requests directly to your AWS Lambda functions, in place of an AWS ALB or AWS API Gateway.

### 

To use Gloo Edge in place of your AWS ALB or AWS API Gateway, you configure the `unwrapAsAlb` setting or the `unwrapAsApiGateway` setting (Gloo Edge Enterprise only, version 1.12.0 or later) in the [AWS `destinationSpec`]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/options/aws/aws.proto.sk/" %}}) of the route to your Lambda upstream. These settings allow Gloo Edge to manipulate a response from an upstream Lambda in the same way as an AWS ALB or AWS API Gateway.



For more information, see the AWS Lambda documentation on [configuring Lambda functions as targets](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/lambda-functions.html) and [how AWS API Gateways process Lambda responses](https://docs.aws.amazon.com/lambda/latest/dg/services-apigateway.html#apigateway-types-transforms).


The following sections walk you through these general steps to set up routing to your Lambda function:
1. Create an AWS Lambda function that returns a response in the form required by the AWS API Gateway.
2. Create a secret containing AWS account credentials that enable access to the Lambda function.
3. Create an Upstream resource that references the Lambda secret.
4. Create a VirtualService resource containing a route action that points to the AWS Lambda upstream.

## Before you begin

* [Install Gloo Edge version 1.12.0 or later in a Kubernetes cluster]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}) or [upgrade your existing installation to version 1.12.0 or later]({{% versioned_link_path fromRoot="/operations/upgrading/upgrade_steps/" %}}).
* The following steps require you to use the access key and secret key for your AWS account. Ensure that the credentials for your AWS account have appropriate permissions to interact with AWS Lambda. For more information, see the [AWS credentials documentation](https://docs.aws.amazon.com/general/latest/gr/aws-sec-cred-types.html).

## Step 1: Create an AWS Lambda

Create an AWS Lambda function that returns a response in the form required by the AWS API Gateway.

1. Log into the AWS console and navigate to the Lambda page.
   
2. Note of your region, which is used when configuring AWS credentials in subsequent steps.

3. Click the **Create Function** button.

4. Name the function `echo` and select `Node.js 16.x` for the runtime.

5. Replace the default contents of `index.js` with the following Node.js Lambda, which returns a response body that contains exactly what was sent to the function in the request body.
   ```js
   exports.handler = async (event) => {
       return event;
   };
   ```

## Step 2: Create an AWS credentials secret

Create a Kubernetes secret that contains the AWS access key and secret key so that Gloo Edge can connect to AWS Lambda for service discovery.

1. Get the access key and secret key for your AWS account. Note that your AWS credentials must have the appropriate permissions to interact with AWS Lambda.

2. Create a Kubernetes secret that contains the AWS access key and secret key.
   ```sh
   glooctl create secret aws \
       --name 'aws-creds' \
       --namespace gloo-system \
       --access-key '$ACCESS_KEY' \
       --secret-key '$SECRET_KEY'
   ```

## Step 3: Create an upstream and virtual service

Create Gloo Edge `Upstream` and `VirtualService` resources to route requests to the Lambda function.

1. Create an upstream resource that references the Lambda secret. Update the region as needed.
   {{< tabs >}}
   {{< tab name="kubectl" codelang="shell">}}
   kubectl apply -f - <<EOF
   apiVersion: gloo.solo.io/v1
   kind: Upstream
   metadata:
     name: aws-upstream
     namespace: gloo-system
   spec:
     aws:
       region: us-east-1
       secretRef:
         name: aws-creds
         namespace: gloo-system
   EOF
   {{< /tab >}}
   {{< tab name="glooctl" codelang="shell">}}
   glooctl create upstream aws \
       --name 'aws-upstream' \
       --namespace 'gloo-system' \
       --aws-region 'us-east-1' \
       --aws-secret-name 'aws-creds' \
       --aws-secret-namespace 'gloo-system'
   {{< /tab >}}
   {{< /tabs >}}

2. Verify that Gloo Edge can access AWS Lambda via your AWS credentials. In the `spec.aws.lambdaFunctions` section of the output, verify that the `echo` Lambda function is listed.
   ```sh
   kubectl get upstream -n gloo-system aws-upstream -o yaml
   ```

3. Create a VirtualService resource containing a `routeAction` that points to the AWS Lambda upstream.
   ```yaml
   kubectl apply -f - <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: aws-route
     namespace: gloo-system
   spec:
     virtualHost:
       domains:
       - '*'
       routes:
       - matchers:
         - exact: /
         routeAction:
           single:
             destinationSpec:
               aws:
                 logicalName: echo
             upstream:
               name: aws-upstream
               namespace: gloo-system
   EOF
   ```

4. Verify that Gloo Edge is routing traffic requests to the Lambda function.
   ```sh
   curl $(glooctl proxy url)/ -d '{"key1":"value1", "key2":"value2"}' -X POST
   ```
   The funtion returns the request body that was sent to it, such as the following:
   ```json
   {"key1":"value1", "key2":"value2"}
   ```



Note that only one setting should be configured. If you configure both, the `unwrapAsAlb` setting is used by default.