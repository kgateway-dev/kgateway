---
title: AWS Lambda
weight: 100
description: Routing to AWS Lambda as an Upstream
---

## How to setup and use AWS Upstream

There are 2 steps to enabling Gloo Edge to discover and access AWS Lambda services.

1. Create an AWS Secret to give Gloo Edge credentials to access AWS.
2. Create a Gloo Edge upstream, referencing AWS Secret, that will populate the Gloo Edge function catalog with available
AWS Lambda functions. 

### Create AWS Secret

The following command will create a Kubernetes secret that contains the AWS Access Key and Secret Key needed by Gloo Edge
to connect to AWS for service discovery.

```shell
glooctl create secret aws --help

Create an AWS secret with the given name

Usage:
  glooctl create secret aws [flags]

Flags:
      --access-key string   aws access key
  -h, --help                help for aws
      --name string         name of the resource to read or write
  -n, --namespace string    namespace for reading or writing resources (default "gloo-system")
      --secret-key string   aws secret key

Global Flags:
  -i, --interactive     use interactive mode
  -o, --output string   output format: (yaml, json, table)
```

For example, to create an AWS secret named `my-aws` in the (default) namespace `gloo-system`, run the following command.
You can name the secret (`--name 'your_name'`) whatever you like. Just make sure you use the correct name when
referencing it from AWS Upstream.

```shell 
glooctl create secret aws \
    --name 'my-aws' \
    --namespace gloo-system \
    --access-key '<AWS ACCESS KEY>' \
    --secret-key '<AWS SECRET KEY>'
```

You can see the details of the created secret as follows.

```shell
kubectl describe secret my-aws -n gloo-system
```

```noop
Name:         my-aws
Namespace:    gloo-system
Labels:       <none>
Annotations:  <none>

Type:  Opaque

Data
====
aws_access_key_id:      20 bytes
aws_secret_access_key:  40 bytes
```

### Create AWS Upstream

This is how you create an AWS Upstream so that Gloo Edge can do both: Lambda service discovery; and allow you to create routing rules
referencing those Lambda functions.

```shell
glooctl create upstream aws --help

AWS Upstreams represent a set of AWS Lambda Functions for a Region that can be routed to with Gloo Edge. AWS Upstreams require a valid set of AWS Credentials to be provided. These should be uploaded to Gloo Edge using `glooctl create secret aws`

Usage:
  glooctl create upstream aws [flags]

Flags:
      --aws-region string                                       region for AWS services this upstream utilize (default "us-east-1")
      --aws-secret-name glooctl create secret aws --help        name of a secret containing AWS credentials created with glooctl. See glooctl create secret aws --help for help creating secrets
      --aws-secret-namespace glooctl create secret aws --help   namespace where the AWS secret lives. See glooctl create secret aws --help for help creating secrets (default "gloo-system")
  -h, --help                                                    help for aws
      --name string                                             name of the resource to read or write
  -n, --namespace string                                        namespace for reading or writing resources (default "gloo-system")

Global Flags:
  -i, --interactive     use interactive mode
  -o, --output string   output format: (yaml, json, table)
```

For example, to create an AWS Upstream nammed `my-aws-upstream` in the (default) namespace `gloo-system` against the AWS
region `us-east-1` and referencing the AWS Secret we created in the previous step - `my-aws` in `gloo-system` namespace.

```shell
glooctl create upstream aws \
    --name 'my-aws-upstream' \
    --namespace 'gloo-system' \
    --aws-region 'us-east-1' \
    --aws-secret-name 'my-aws' \
    --aws-secret-namespace 'gloo-system'
```

### Usage

To create a route rule for your new AWS upstream, you use the `glooctl add route` command with the `--aws-function-name`
option. For example,

```shell
glooctl add route \
    --name 'default' \
    --namespace 'gloo-system' \
    --path-prefix '/helloworld' \
    --dest-name 'my-aws' \
    --aws-function-name 'helloworld'
```

