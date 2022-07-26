---
title: AWS Lambda
weight: 100
description: Routing to AWS Lambda as an Upstream
---

Route traffic requests directly to an [Amazon Web Services (AWS) Lambda function](https://aws.amazon.com/lambda/resources/).

## About

Gloo Edge enables you to route traffic requests directly to your AWS Lambda functions, in place of an AWS ALB or AWS API Gateway.

To use Gloo Edge in place of your AWS ALB or AWS API Gateway, you configure the `unwrapAsAlb` setting or the `unwrapAsApiGateway` setting (Gloo Edge Enterprise only, version 1.12.0 or later) in the [AWS `destinationSpec`]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/options/aws/aws.proto.sk/" %}}) of the route to your Lambda upstream. These settings allow Gloo Edge to manipulate a response from an upstream Lambda in the same way as an AWS ALB or AWS API Gateway.

Gloo Edge looks for a JSON response from the Lambda upstream that contains the following specific fields:
- `body`: String containing the desired response body.
- `headers`: JSON object containing a mapping from the desired response header keys to the desired response header values.
- `multiValueHeaders`: JSON object containing a mapping from the desired response header keys to a list of the desired response header values to be mapped to that header key.
- `statusCode`: Integer representing the desired HTTP response status code (default `200`).
- `isBase64Encoded`: Boolean for whether to decode the provided body string as base64 (default `false`).

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

3. Create a VirtualService resource containing a `routeAction` that points to the AWS Lambda upstream. In the `destinationSpec.aws` section, include one of the following settings. Note that only one setting should be configured. If you configure both, the `unwrapAsAlb` setting is used by default.
   * `unwrapAsAlb: true`: Replace the functionality of an AWS ALB.
   * `unwrapAsApiGateway: true` (Gloo Edge Enterprise only, version 1.12.0 or later): Replace the functionality of an AWS API Gateway.
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
                 unwrapAsApiGateway: true
             upstream:
               name: aws-upstream
               namespace: gloo-system
   EOF
   ```

4. Verify that Gloo Edge correctly routes traffic requests to the Lambda function.
   ```sh
   curl $(glooctl proxy url)/ -d '{"body": "gloo edge is inserting this body", "headers": {"test-header-key": "test-header-value"}, "statusCode": 201}' -X POST -v
   ```
   A successful response contains the same body string, response headers, and status code that you provided in the curl command, such as the following:
   ```
   *   Trying ::1...
   * TCP_NODELAY set
   * Connected to localhost (::1) port 8080 (#0)
   > POST / HTTP/1.1
   > Host: localhost:8080
   > User-Agent: curl/7.64.1
   > Accept: */*
   > Content-Length: 116
   > Content-Type: application/x-www-form-urlencoded
   > 
   * upload completely sent off: 116 out of 116 bytes
   < HTTP/1.1 201 Created
   < test-header-key: test-header-value
   < content-length: 32
   < date: Mon, 25 Jul 2022 13:37:05 GMT
   < server: envoy
   < 
   * Connection #0 to host localhost left intact
   gloo edge is inserting this body* Closing connection 0
   ```