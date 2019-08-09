---
title: "glooctl create upstream aws"
weight: 5
---
## glooctl create upstream aws

Create an Aws Upstream

### Synopsis

AWS Upstreams represent a set of AWS Lambda Functions for a Region that can be routed to with Gloo. AWS Upstreams require a valid set of AWS Credentials to be provided. These should be uploaded to Gloo using `glooctl create secret aws`

```
glooctl create upstream aws [flags]
```

### Options

```
      --aws-region string                                       region for AWS services this upstream utilize (default "us-east-1")
      --aws-secret-name glooctl create secret aws --help        name of a secret containing AWS credentials created with glooctl. See glooctl create secret aws --help for help creating secrets
      --aws-secret-namespace glooctl create secret aws --help   namespace where the AWS secret lives. See glooctl create secret aws --help for help creating secrets (default "gloo-system")
  -h, --help                                                    help for aws
```

### Options inherited from parent commands

```
      --dry-run             print kubernetes-formatted yaml rather than creating or updating a resource
  -i, --interactive         use interactive mode
      --name string         name of the resource to read or write
  -n, --namespace string    namespace for reading or writing resources (default "gloo-system")
  -o, --output OutputType   output format: (yaml, json, table, kube-yaml) (default table)
```

### SEE ALSO

* [glooctl create upstream](../glooctl_create_upstream)	 - Create an Upstream

