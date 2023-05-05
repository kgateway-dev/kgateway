---
title: Using Cross-Account Lambda Functions
weight: 101
description: |
  This guide describes how to configure Gloo Edge to route to AWS Lambda functions in different accounts than the one used to authenticate with AWS.
---

# Overview
 - Gloo Edge supports routing to AWS Lambda functions in different accounts than the one used to authenticate with AWS
 - There are two strategies that can be used to configure this behavior:
   1. [IAM Roles for Service Accounts/Role-Chained Configuration](#iam-roles-for-service-accounts-irsa-configuration) (Recommended)
   2. [Resource-Based Configuration](#resource-based-configuration)

# IAM Roles for Service Accounts/Role-Chained Configuration
 - This is the recommended workflow for using cross-account Lambda functions with Gloo Edge
 - The configuration is essentially a modified version of the AWS Lambda with EKS ServiceAccounts [guide]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_types/aws_lambda/eks-service-accounts/" >}})
## AWS Configuration
- For this configuration you will need to create two roles, one in the primary account and one in the target account. The primary account is the account that you will use to authenticate with AWS, and the target account is the account that contains the Lambda functions that you wish to route to. The role in the primary account will be used to assume the role in the target account, which will be used to invoke the Lambda function.
### Primary Account
  - Follow the steps in the [AWS Lambda with EKS ServiceAccounts guide]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_types/aws_lambda/eks-service-accounts/" >}})
    - As part of this guide, you will create a role which will be associated with the ServiceAccount in your cluster. This role will be used to assume the role in the target account, which will be used to invoke the Lambda function. Make sure to note the `ARN` of this role to use when configuring the target account's role later.
### Target Account
 - Create a Lambda function in the target account
 - Create a role which:
   1. Can be used to invoke the Lambda function, which requires the `lambda:InvokeFunction` permission
   1. Can be assumed by the role associated with the ServiceAccount in the primary account
     - To do so, specify the following trust policy on the role with the `ARN` of the primary account's role that you previously retrieved:
       ```json
        {
            "Version": "2012-10-17",
            "Statement": [
                {
                    "Effect": "Allow",
                    "Principal": {
                        "AWS": <ARN OF PRIMARY ACCOUNT ROLE>,
                    },
                    "Action": "sts:AssumeRole",
                    "Condition": {}
                }
            ]
        }
       ```
    
## Gloo Edge Configuration
 - Disable Function Discovery (FDS), which automatically discovers functions in your primary AWS account. Because you want to route to Lambdas in more than one account, you must disable discovery and instead manually configure the functions.
   - This can be done via the `gloo.discovery.fdsMode` setting to `DISABLED` in the enterprise Helm chart
   - Alternatively, you can set `spec.discovery.fdsMode` to `DISABLED` in the `gloo.solo.io/v1.Settings` custom resource
 - Specify the Lambda functions in the target account that you wish to route to on the upstream
   - These are defined under the `spec.aws.LambdaFunctions` field of the upstream
- Configure the `spec.aws.roleArn` field of the upstream to point to the IAM role that will be used to invoke the Lambda functions, which is the role that you created in the target account
  - This role should have the `lambda:InvokeFunction` permission
  - Example Upstream:
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
          roleArn: arn:aws:iam::123456789012:role/lambda-role
          # because we have disabled FDS, we need to hardcode the lambda functions in the upstream spec
          lambdaFunctions:
          - lambdaFunctionName: target-name
            logicalName: target-name
      ```

# Resource-Based Configuration
## AWS Configuration
- For this configuration you will need to create a user or role in the primary account, and a Lambda function in the target account. The Lambda function will have a resource-based policy statement which will allow the user or role in the primary account to invoke it.
### Primary Account
 - Create a user or role in the primary account that will be used to invoke the Lambda functions in the secondary account
   - Give the user the `lambda:InvokeFunction` permission
   - Create an access key for the user. This will be used to authenticate with AWS when invoking the Lambda functions
### Target Account
 - Create a Lambda function in the target account
 - Define a Resource-based policy statement for the function, which will allow the user in the Primary account to invoke it
   - In the AWS console, select the Lambda function, and click the "Configuration" tab
   - From there, click the "Permissions" tab in the sidebar, and scroll down to the "Resource-based policy statements" section
   - Click "Add Permissions", and select "AWS account" as the entity which will invoke the function
   - Use the ARN of the user or role in the primary account as the principal
   - Select `lambda:InvokeFunction` as the action
## Gloo Edge Configuration
 - Disable Function Discovery (FDS). This can be used to automatically discover functions in the user's primary AWS account, but since we are using a secondary account, we will need to manually configure the functions.
   - This can be done via the `gloo.discovery.fdsMode` setting to `DISABLED` in the enterprise Helm chart
   - Alternatively, you can set `spec.discovery.fdsMode` to `DISABLED` in the `gloo.solo.io/v1.Settings` custom resource
 - Configure an AWS secret, using the access key and secret key of the user in the primary account
   - See the [AWS Lambda guide]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_types/aws_lambda/" >}}) for more information on how to do this
   - This secret will be used to authenticate with AWS when invoking the Lambda functions
 - Specify the Lambda functions in the target account that you wish to route to on the upstream
   - These are defined under the `spec.aws.LambdaFunctions` field of the upstream 
 - Specify the target account ID in the upstream
   - This is defined under the `spec.aws.awsAccountId` field of the upstream
 - Example Upstream:
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
         # because we have disabled FDS, we need to hardcode the lambda functions in the upstream spec
         lambdaFunctions:
         - lambdaFunctionName: target-name
           logicalName: target-name
     ```
## Validation
 - To validate that the configuration is correct, you can follow the steps in the [AWS Lambda guide]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_types/aws_lambda#step-3-create-an-upstream-and-virtual-service" >}}) to create a virtual service that routes to the Lambda function via the AWS upstream