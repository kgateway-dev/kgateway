---
title: "glooctl gateway-api convert"
description: "Reference for the 'glooctl gateway-api convert' command."
weight: 5
---
## glooctl gateway-api convert

Convert Gloo Edge APIs to Gateway API

### Synopsis

Convert Gloo Edge APIs to Gateway API by either providing kubernetes yaml files or a Gloo Gateway input snapshot

```
glooctl gateway-api convert [flags]
```

### Examples

```
# This command converts Gloo Edge APIs to Gloo Gateway API yaml and places them in the '--output-dir' directory arranged by namespace.
# To generate gateway api by a single kubernetes yaml file. The 'output-dir'' must not exist
  glooctl gateway-api convert --input-file gloo-yamls.yaml --output-dir ./_output

# To delete and recreate the output dir add 'delete-output-dir'
  glooctl gateway-api convert --input-file gloo-yamls.yaml --output-dir ./_output --delete-output-dir

# To generate gateway api by a single kubernetes yaml file but place all the output configurations in the same file.
  glooctl gateway-api convert --input-file gloo-yamls.yaml --output-dir ./_output --retain-input-folder-structure

# To load a bunch of *.yaml or *.yml files in nested directories. You can also retain the same file structure which is helpful in CI/CD pipelines.
  glooctl gateway-api convert --input-dir ./gloo-configs --output-dir ./_output --retain-input-folder-structure

# Configuration can also be pulled an translated directly from a running Gloo Gateway Instance (version 1.17+).
# Port forward to the running gloo instance to grab its snapshot
  kubectl -n gloo-system port-forward deploy/gloo 9091
  curl localhost:9091/snapshots/input > gg-input.json
  
  glooctl gateway-api convert --input-snapshot gg-input.json --output-dir ./_output

# For all commands you can print stats about the migration such as number of configs
  glooctl gateway-api convert --input-file gloo-yamls.yaml --output-dir ./_output --print-stats

# If the yaml files contain non Gloo API yaml they can be retained by adding '--include-unknown'
  glooctl gateway-api convert --input-file gloo-yamls.yaml --output-dir ./_output --include-unknown
```

### Options

```
      --combine-route-options           Combine routeOptions that are exactly the same and share them among the HTTPRoutes
      --delete-output-dir               Delete the output directory if it already exists
  -h, --help                            help for convert
      --include-unknown                 Copy unknown resources to output files (if files contain resources that are not Gloo APIs)
      --input-dir string                InputDir to read yaml/yml files recursively
      --input-file string               Convert single file to Gateway API
      --input-snapshot string           Gloo input snapshot file location
      --output-dir string               Output directory to write Gateway API configurations, it must not exist before or can be deleted/recreated with --recreate-output-dir (default "./_output")
      --print-stats                     Print stats about the conversion
      --retain-input-folder-structure   When writing the output write the Gateway API configurations in the same folder structure they were read from (input-dir only)
```

### Options inherited from parent commands

```
  -c, --config string              set the path to the glooctl config file (default "<home_directory>/.gloo/glooctl-config.yaml")
      --consul-address string      address of the Consul server. Use with --use-consul (default "127.0.0.1:8500")
      --consul-allow-stale-reads   Allows reading using Consul's stale consistency mode.
      --consul-datacenter string   Datacenter to use. If not provided, the default agent datacenter is used. Use with --use-consul
      --consul-root-key string     key prefix for the Consul key-value storage. (default "gloo")
      --consul-scheme string       URI scheme for the Consul server. Use with --use-consul (default "http")
      --consul-token string        Token is used to provide a per-request ACL token which overrides the agent's default token. Use with --use-consul
  -i, --interactive                use interactive mode
      --kube-context string        kube context to use when interacting with kubernetes
      --kubeconfig string          kubeconfig to use, if not standard one
      --use-consul                 use Consul Key-Value storage as the backend for reading and writing config (VirtualServices, Upstreams, and Proxies)
```

### SEE ALSO

* [glooctl gateway-api](../glooctl_gateway-api)	 - Gateway API specific commands

