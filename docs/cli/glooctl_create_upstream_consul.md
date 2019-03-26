---
title: "glooctl create upstream consul"
weight: 5
---
## glooctl create upstream consul

Create a Consul Upstream

### Synopsis

Consul Upstreams represent a collection of endpoints for Services registered with Consul. Typically, Gloo will automatically discover these upstreams, meaning you don't have to create them. However, if upstream discovery in Gloo is disabled, or ACL permissions have not been granted to Gloo to read from the registry, Consul services can be added to Gloo manually via the CLI.

```
glooctl create upstream consul [flags]
```

### Options

```
      --consul-service string         name of the service in the consul registry
      --consul-service-tags strings   comma-separated list of tags for choosing a subset of the service in the consul registry
  -h, --help                          help for consul
      --service-spec-type string      if set, Gloo supports additional routing features to upstreams with a service spec. The service spec defines a set of features 
```

### Options inherited from parent commands

```
  -i, --interactive        use interactive mode
  -k, --kubeyaml           print kubernetes-formatted yaml rather than creating or updating a resource
      --name string        name of the resource to read or write
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
  -o, --output string      output format: (yaml, json, table)
```

### SEE ALSO

* [glooctl create upstream](../glooctl_create_upstream)	 - Create an Upstream Interactively

