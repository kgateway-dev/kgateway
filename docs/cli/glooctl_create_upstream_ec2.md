---
title: "glooctl create upstream ec2"
weight: 5
---
## glooctl create upstream ec2

Create an EC2 Upstream

### Synopsis

EC2 Upstreams represent a collection of EC2 instance endpoints that match the specified tag filters. You can use private (default) or public IP addresses and and any port on the instance (default: 80).EC2 Upstreams require a valid set of AWS Credentials to be provided, either as an AWS secret, or in the environment. You can optionally provide a Role for Gloo to assume on behalf of this upstream.

```
glooctl create upstream ec2 [flags]
```

### Options

```
      --aws-region string                                       region for AWS services this upstream utilize (default "us-east-1")
      --aws-role-arn string                                     Amazon Resource Number (ARN) of role that Gloo should assume on behalf of the upstream
      --aws-secret-name glooctl create secret aws --help        name of a secret containing AWS credentials created with glooctl. See glooctl create secret aws --help for help creating secrets
      --aws-secret-namespace glooctl create secret aws --help   namespace where the AWS secret lives. See glooctl create secret aws --help for help creating secrets (default "gloo-system")
      --ec2-port uint32                                         port to use to connect to the EC2 instance (default 80) (default 80)
  -h, --help                                                    help for ec2
      --public-ip                                               use instance's public IP address
      --tag-key-filters strings                                 list of tag keys that must exist on EC2 instances associated with this upstream
      --tag-key-value-filters strings                           list of tag keys and corresponding values that must exist on EC2 instances associated with this upstream
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

