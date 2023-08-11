---
title: Using Cross-Account Lambda Functions
weight: 101
description: |
  This guide describes how to configure Gloo Edge to route to AWS Lambda functions in different accounts than the one used to authenticate with AWS.
---

Route to AWS Lambda functions that exist in different accounts than the account you use to authenticate with AWS.

You can set up multi-account routing in the following ways:
* [IAM Roles for Service Accounts/Role-chained configuration (recommended)](#irsa)
* [Resource-based configuration](#resource-based-configuration)

## IAM Roles for Service Accounts/Role-chained configuration {#irsa}

Use AWS IAM Roles for Service Accounts (IRSA) to configure routing to functions in different accounts. This method is recommended for using cross-account Lambda functions with Gloo Edge.

### AWS configuration

Create roles in your authentication and Lambda AWS accounts. In the account that you want to use to authenticate with AWS, you create a role that is used to assume the role in the Lambda account. In the account that contains the Lambda functions you want to route to, you create a role that is used to invoke the Lambda functions.

1. In your authentication account, create a role by following the steps in the [AWS Lambda with EKS ServiceAccounts guide]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_types/aws_lambda/eks-service-accounts/" >}}). In this guide, you create a role that is associated with the ServiceAccount in your cluster. Make sure to note the ARN of this role, which you use in subsequent steps to create the Lambda account's role.

2. In your Lambda account, create the following resources.
   1. Choose an existing or create a new Lambda function that you want to route to.
   2. Create an IAM policy that contains at least the `lambda:InvokeFunction` permission to allow Lambda function invocation, such as the following example.
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
   3. Create an IAM role that uses your invocation policy. In the role, specify the ARN of the authentication account's role that you created in step 1, such as the following example. After you create this role, make sure to note the role's ARN, which you specify in a Gloo Edge upstream resource in subsequent steps.
      ```json
      {
          "Version": "2012-10-17",
          "Statement": [
              {
                  "Effect": "Allow",
                  "Principal": {
                      "AWS": "<ARN of authentication account role>"
                  },
                  "Action": "sts:AssumeRole",
                  "Condition": {}
              }
          ]
      }
      ```
   
### Gloo Edge configuration

Modify your Gloo Edge installation settings and upstream resources to support routing to the Lambda functions.

1. Disable Function Discovery (FDS), which automatically discovers functions in your authentication account. You can disable FDS in one of the following ways:
   - In the Gloo Edge Enterprise Helm chart, set `gloo.discovery.fdsMode` to `DISABLED`.
   - In the `gloo.solo.io/v1.Settings` custom resource, set `spec.discovery.fdsMode` to `DISABLED`.

2. Create an upstream resource for each Lambda function that you want to route to.
   - In the `spec.aws.roleArn` field, specify the IAM role ARN that is used to invoke the Lambda functions, which you created for the Lambda account in step 2.3 of the previous section.
   - In the `spec.aws.lambdaFunctions` section, specify the Lambda function details.
   ```yaml
   apiVersion: gloo.solo.io/v1
   kind: Upstream
   metadata:
     name: aws-upstream
     namespace: gloo-system
   spec:
     aws:
       region: us-east-1
       roleArn: arn:aws:iam::123456789012:role/lambda-role
       lambdaFunctions:
       - lambdaFunctionName: target-name
         logicalName: target-name
   ```
3. Optional: [Verify routing](#verify-routing).

## Resource-based configuration

Use AWS resource-based configuration to configure routing to functions in different accounts.
### AWS configuration

For the AWS configuration, you create a user or role in the authentication account, and a Lambda function in the account that contains the Lambda functions. The Lambda function has a resource-based policy statement which allows the user or role in the authentication account to invoke it.

1. Create the following resources for your authentication account. 
   1. Create a user or role in your authentication account. Be sure to give the user or role the `lambda:InvokeFunction` permission, so that the role can used to invoke the Lambda functions in the other account.
   2. Create an access key for the user or role, which is used to authenticate with AWS when invoking the Lambda functions.
   3. Create a Kubernetes secret that contains the access key and secret key.
      ```sh
      glooctl create secret aws \
          --name 'aws-creds' \
          --namespace gloo-system \
          --access-key $ACCESS_KEY \
          --secret-key $SECRET_KEY
      ```

2. Create the following resources for your Lambda account.
   1. Choose an existing or create a new Lambda function that you want to route to.
   2. Define a resource-based policy statement for the function, which allows the user in the authentication account to invoke it.
      1. In the AWS console, select the Lambda function.
      2. Click the **Configuration** tab.
      3. In the sidebar, click the **Permissions** tab.
      4. In the **Resource-based policy statements** section, click **Add Permissions**.
         * Select **AWS account** as the entity which invokes the function.
         * Specify the ARN of the user or role in the authentication account as the principal.
         * Select `lambda:InvokeFunction` as the action.

### Gloo Edge configuration

Modify your Gloo Edge installation settings and upstream resources to support routing to the Lambda functions.

1. Disable Function Discovery (FDS), which automatically discovers functions in your authentication account. You can disable FDS in one of the following ways:
   - In the Gloo Edge Enterprise Helm chart, set `gloo.discovery.fdsMode` to `DISABLED`.
   - In the `gloo.solo.io/v1.Settings` custom resource, set `spec.discovery.fdsMode` to `DISABLED`.

2. Create an upstream resource for each Lambda function that you want to route to.
   - In the `spec.aws.awsAccountId` field, specify the ID for the account that contains the Lambda functions.
   - In the `spec.aws.lambdaFunctions` section, specify the Lambda function details.
   ```yaml
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
       awsAccountId: "123456789012"
       lambdaFunctions:
       - lambdaFunctionName: target-name
         logicalName: target-name
   ```
3. Optional: [Verify routing](#verify-routing).

## Verify routing

To verify that the configuration is correct, you can follow the steps in the [AWS Lambda guide]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_types/aws_lambda#step-3-create-an-upstream-and-virtual-service" >}}) to create a virtual service that routes to the Lambda function via the AWS upstream.
