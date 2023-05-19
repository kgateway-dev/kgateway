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
  -c, --config string              set the path to the glooctl config file (default "<home_directory>/.gloo/glooctl-config.yaml")
      --consul-address string      address of the Consul server. Use with --use-consul (default "127.0.0.1:8500")
      --consul-allow-stale-reads   Allows reading using Consul's stale consistency mode.
      --consul-datacenter string   Datacenter to use. If not provided, the default agent datacenter is used. Use with --use-consul
      --consul-root-key string     key prefix for for Consul key-value storage. (default "gloo")
      --consul-scheme string       URI scheme for the Consul server. Use with --use-consul (default "http")
      --consul-token string        Token is used to provide a per-request ACL token which overrides the agent's default token. Use with --use-consul
      --dry-run                    print kubernetes-formatted yaml rather than creating or updating a resource
  -i, --interactive                use interactive mode
      --kube-context string        kube context to use when interacting with kubernetes
      --kubeconfig string          kubeconfig to use, if not standard one
      --name string                name of the resource to read or write
  -n, --namespace string           namespace for reading or writing resources (default "gloo-system")
  -o, --output OutputType          output format: (yaml, json, table, kube-yaml, wide) (default table)
      --use-consul                 use Consul Key-Value storage as the backend for reading and writing config (VirtualServices, Upstreams, and Proxies)
```

### SEE ALSO

* [glooctl create upstream](../glooctl_create_upstream)	 - Create an Upstream

