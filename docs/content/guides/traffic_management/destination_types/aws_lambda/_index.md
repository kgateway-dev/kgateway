---
title: AWS Lambda
weight: 100
description: Routing to AWS Lambda as an Upstream
---

Route traffic requests directly to an [Amazon Web Services (AWS) Lambda function](https://aws.amazon.com/lambda/resources/).

## About

Gloo Edge enables you to route traffic requests directly to your AWS Lambda functions. To also use Gloo Edge in place of your AWS ALB or AWS API Gateway, you can configure the `unwrapAsAlb` or `unwrapAsApiGateway` setting (Gloo Edge Enterprise only, version 1.12.0 or later) in the [AWS `destinationSpec`]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/options/aws/aws.proto.sk/" %}}) of the route to your Lambda upstream. These settings allow Gloo Edge to manipulate a response from an upstream Lambda in the same way as an AWS ALB or AWS API Gateway.

For more information, see the AWS Lambda documentation on [configuring Lambda functions as targets](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/lambda-functions.html).

## Set up routing to AWS Lambda

**Before you begin**: The following steps require you to use the access key and secret key for your AWS account. Ensure that the credentials for your AWS account have appropriate permissions to interact with AWS Lambda. For more information, see the [AWS credentials documentation](https://docs.aws.amazon.com/general/latest/gr/aws-sec-cred-types.html).

### Step 1: Create an AWS Lambda

Create an AWS Lambda function to test with Gloo Edge routing.

1. Log in to the AWS console and navigate to the Lambda page.

2. Note your region, which is used when configuring the AWS upstream in subsequent steps.

3. Click the **Create Function** button.

4. Name the function `echo` and select `Node.js 16.x` for the runtime.

5. Replace the default contents of `index.js` with the following Node.js Lambda, which returns a response body that contains exactly what was sent to the function in the request body.
   ```js
   exports.handler = async (event) => {
       return event;
   };
   ```

### Step 2: Create an AWS credentials secret

Create a Kubernetes secret that contains your AWS access key and secret key. Gloo Edge uses this secret to connect to AWS Lambda for service discovery.

1. Get the access key and secret key for your AWS account. Note that your [AWS credentials](https://docs.aws.amazon.com/general/latest/gr/aws-sec-cred-types.html) must have the appropriate permissions to interact with AWS Lambda.

2. Create a Kubernetes secret that contains the AWS access key and secret key.
   ```sh
   glooctl create secret aws \
       --name 'aws-creds' \
       --namespace gloo-system \
       --access-key $ACCESS_KEY \
       --secret-key $SECRET_KEY
   ```

### Step 3: Create an upstream and virtual service

Create Gloo Edge `Upstream` and `VirtualService` resources to route requests to the Lambda function.

1. Create an upstream resource that references the Lambda secret. Update the region with your Lamda location, such as `us-east-1`.
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
       region: <region>
       secretRef:
         name: aws-creds
         namespace: gloo-system
   EOF
   {{< /tab >}}
   {{< tab name="glooctl" codelang="shell">}}
   glooctl create upstream aws \
       --name 'aws-upstream' \
       --namespace 'gloo-system' \
       --aws-region '<region>' \
       --aws-secret-name 'aws-creds' \
       --aws-secret-namespace 'gloo-system'
   {{< /tab >}}
   {{< /tabs >}}

2. Verify that Gloo Edge can access AWS Lambda via your AWS credentials. In the `spec.aws.lambdaFunctions` section of the output, verify that the `echo` Lambda function is listed.
   ```sh
   kubectl get upstream -n gloo-system aws-upstream -o yaml
   ```

3. Create a VirtualService resource containing a `routeAction` that points to the AWS Lambda upstream. Note that this resource directs Gloo Edge to route requests directly to the Lambda upstram, but does not manipulate the JSON response.
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

4. Confirm that Gloo Edge is correctly routing requests to Lambda by sending a curl request.
   ```sh
   curl $(glooctl proxy url)/ -d '{"key1":"value1", "key2":"value2"}' -X POST
   ```
   Example response:
   ```json
   {"key1":"value1", "key2":"value2"}
   ```

At this point, Gloo Edge is routing directly to the `echo` Lambda function. To configure Gloo Edge to also unwrap the JSON response from the function in the same way as an AWS ALB or AWS API Gateway, continue to the next section.


## Transform requests and/or responses

### Basic request transformations

When you use the AWS Lambda plug-in in Gloo Edge, you might want to transform requests to increase the amount of information passed to the Lambda function.

The request transformation injects the headers of the request into the body. The resulting body will be a JSON object containing two keys: 'headers', which contains the injected headers, and 'body', which contains the original body.

Note that request transformations are incompatible with the `wrapAsApiGateway` setting, which transforms requests into the format produced by an AWS API Gateway.

**Before you begin**: [Install Gloo Edge version 1.12.0 or later in a Kubernetes cluster]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}) or [upgrade your existing installation to version 1.12.0 or later]({{% versioned_link_path fromRoot="/operations/upgrading/upgrade_steps/" %}}).

1. Edit the VirtualService resource that you created in the previous section to add the `destinationSpec.aws.requestTransformation: true` setting.
   ```bash
   kubectl edit virtualservices.gateway.solo.io -n gloo-system aws-route
   ```
   {{< highlight yaml "hl_lines=5-8" >}}
   ...
         routeAction:
           single:
             destinationSpec:
               aws:
                 logicalName: echo
                 requestTransformation: true
             upstream:
               name: aws-upstream
               namespace: gloo-system
   {{< / highlight >}}


### Unwrap responses as an AWS ALB

Unwrap the JSON response from the function in the same way as an AWS ALB.

In the following steps, you configure the `unwrapAsAlb` setting in the [AWS `destinationSpec`]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/options/aws/aws.proto.sk/" %}}) of the route to your Lambda upstream. These settings allow Gloo Edge to manipulate a response from an upstream Lambda in the same way as an AWS ALB .

1. Edit the VirtualService resource that you created in the previous section to add the `destinationSpec.aws.unwrapAsAlb: true` setting.
   ```bash
   kubectl edit virtualservices.gateway.solo.io -n gloo-system aws-route
   ```
   {{< highlight yaml "hl_lines=5-7" >}}
   ...
         routeAction:
           single:
             destinationSpec:
               aws:
                 logicalName: echo
                 unwrapAsAlb: true
             upstream:
               name: aws-upstream
               namespace: gloo-system
   {{< / highlight >}}

2. Verify that Gloo Edge correctly routes traffic requests to the Lambda function and unwraps the response from the function.
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

### Wrap or unwrap requests and responses as an AWS API Gateway (Enterprise v1.12.0+ only)

In Gloo Edge Enterprise, you can use Gloo Edge in place of an AWS API Gateway. To do this, configure the `wrapAsApiGateway` setting or the `unwrapAsApiGateway` setting (Gloo Edge Enterprise only, version 1.12.0 or later) in the [AWS `destinationSpec`]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/options/aws/aws.proto.sk/" %}}) of the route to your Lambda upstream. These settings allow Gloo Edge to manipulate a response from an upstream Lambda in the same way as an AWS API Gateway.

#### Unwrap as API Gateway

Unwrap the JSON response from the function in the same way as an AWS API Gateway.

Note that to use the `unwrapAsApiGateway` setting, your Lambda function must be capable of returning a response in the form that is required by an AWS API Gateway. Gloo Edge looks for a JSON response from the Lambda upstream that contains the following specific fields:
- `body`: String containing the desired response body.
- `headers`: JSON object containing a mapping from the desired response header keys to the desired response header values.
- `multiValueHeaders`: JSON object containing a mapping from the desired response header keys to a list of the desired response header values that you want to map to a header key.
- `statusCode`: Integer representing the desired HTTP response status code (default `200`).
- `isBase64Encoded`: Boolean for whether to decode the provided body string as base64 (default `false`).

For more information, see the AWS Lambda documentation on [how AWS API Gateways process Lambda responses](https://docs.aws.amazon.com/lambda/latest/dg/services-apigateway.html#apigateway-types-transforms).

**Before you begin**: [Install Gloo Edge Enterprise version 1.12.0 or later in a Kubernetes cluster]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}) or [upgrade your existing Enterprise installation to version 1.12.0 or later]({{% versioned_link_path fromRoot="/operations/upgrading/upgrade_steps/" %}}).

1. Edit the VirtualService resource that you created in the previous section to add the `destinationSpec.aws.unwrapAsApiGateway: true` setting.
   ```bash
   kubectl edit virtualservices.gateway.solo.io -n gloo-system aws-route
   ```
   {{< highlight yaml "hl_lines=5-7" >}}
   ...
         routeAction:
           single:
             destinationSpec:
               aws:
                 logicalName: echo
                 unwrapAsApiGateway: true
             upstream:
               name: aws-upstream
               namespace: gloo-system
   {{< / highlight >}}

2. Verify that Gloo Edge correctly routes traffic requests to the Lambda function and unwraps the response from the function.
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

#### Wrap as API Gateway

Wrap the request to the function in the same way as an AWS API Gateway.

**Before you begin**: [Install Gloo Edge Enterprise version 1.12.0 or later in a Kubernetes cluster]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}) or [upgrade your existing Enterprise installation to version 1.12.0 or later]({{% versioned_link_path fromRoot="/operations/upgrading/upgrade_steps/" %}}).

1. Edit the VirtualService resource that you created in the previous section to add the `destinationSpec.aws.unwrapAsApiGateway: true` setting.
   ```bash
   kubectl edit virtualservices.gateway.solo.io -n gloo-system aws-route
   ```
   {{< highlight yaml "hl_lines=5-7" >}}
   ...
         routeAction:
           single:
             destinationSpec:
               aws:
                 logicalName: echo
                 wrapAsApiGateway: true
             upstream:
               name: aws-upstream
               namespace: gloo-system
   {{< / highlight >}}

2. Verify that Gloo Edge correctly routes traffic requests to the Lambda function and unwraps the response from the function.
   ```sh
   curl $(glooctl proxy url)/ -d 'gloo edge is inserting this body' -H 'test-header-key: test-header-value' -X POST -v
   ```
   A successful response contains the body as received by the upstream lambda function, such as the following:
   ```
   *   Trying ::1...
   * TCP_NODELAY set
   * Connected to localhost (::1) port 8080 (#0)
   > POST / HTTP/1.1
   > Host: localhost:8080
   > User-Agent: curl/7.85.0
   > Accept: */*
   > test-header-key: test-header-value
   > Content-Length: 32
   > Content-Type: application/x-www-form-urlencoded
   >
   * Mark bundle as not supporting multiuse
   < HTTP/1.1 200 OK
   < date: Mon, 13 Feb 2023 13:22:06 GMT
   < content-type: application/json
   < content-length: 754
   < x-amzn-requestid: 24c24091-ddf9-4bc5-9799-e86f06ca60b7
   < x-amzn-remapped-content-length: 0
   < x-amz-executed-version: $LATEST
   < x-amzn-trace-id: root=1-63ea397d-75fcad35063993b6357a03ce;sampled=0
   < x-envoy-upstream-service-time: 397
   < server: envoy
   <
   * Connection #0 to host localhost left intact
   {"body": "gloo edge is inserting this body", "headers": {":authority": "localhost:8080", ":method": "POST", ":path": "/", ":scheme": "http", "accept": "*/*", "content-length": "32", "content-type": "application/x-www-form-urlencoded", "test-header-key": "test-header-value", "user-agent": "curl/7.85.0", "x-forwarded-proto": "http", "x-request-id": "347975da-fa61-4d8a-9285-ee0826202819"}, "httpMethod": "POST", "isBase64Encoded": false, "multiValueHeaders": null, "multiValueQueryStringParameters": null, "path": "/", "pathParameters": null, "queryStringParameters": null, "requestContext": {"httpMethod": "POST", "path": "/", "protocol": "HTTP/1.1", "resourcePath": "/"}, "resource": "/", "routeKey": "POST /", "stageVariables": null, "version": "1.0"}%
   ```

