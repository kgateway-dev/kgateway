---
title: "glooctl add route"
weight: 5
---
## glooctl add route

Add a Route to a Virtual Service

### Synopsis

Routes match patterns on requests and indicate the type of action to take when a proxy receives a matching request. Requests can be broken down into their Match and Action components. The order of routes within a Virtual Service matters. The first route in the virtual service that matches a given request will be selected for routing. 

If no virtual service is specified for this command, glooctl add route will attempt to add it to a default virtual service with domain '*'. if one does not exist, it will be created for you.

Usage: `glooctl add route [--name virtual-service-name] [--namespace namespace] [--index x] ...`

```
glooctl add route [flags]
```

### Options

```
      --aws-alb-unwrap                    Sets if gloo should handle responses as if it was an ALB. Appropriately handles the response body and sets headers.
      --aws-api-gw-unwrap                 Sets if gloo should handle responses as if it was an API Gateway. Appropriately handles the response body and sets headers.
  -a, --aws-function-name string          logical name of the AWS lambda to invoke with this route. use if destination is an AWS upstream
      --aws-unescape                      unescape JSON returned by this lambda function (useful if the response is not intended to be JSON formatted, e.g. in the case of static content (images, HTML, etc.) being served by Lambda
      --cluster-scoped-vs-client          search for *-domain virtual services outside gloo system namespace to add route to
      --delegate-name string              name of the delegated RouteTable for this route
      --delegate-namespace string         namespace of the delegated RouteTable for this route (default "gloo-system")
  -u, --dest-name string                  name of the destination upstream for this route
  -s, --dest-namespace string             namespace of the destination upstream for this route (default "gloo-system")
  -d, --header strings                    headers to match on the request. values can be specified using regex strings
  -h, --help                              help for route
  -x, --index uint32                      index in the virtual service's or route table'sroute list where to insert this route. routes after it will be shifted back one
  -m, --method strings                    the HTTP methods (GET, POST, etc.) to match on the request. if empty, all methods will match 
  -o, --output OutputType                 output format: (yaml, json, table, kube-yaml, wide) (default table)
  -e, --path-exact string                 exact path to match route
  -p, --path-prefix string                path prefix to match route
  -r, --path-regex string                 regex matcher for route. note: only one of path-exact, path-regex, or path-prefix should be set
      --prefix-rewrite string             rewrite the matched portion of HTTP requests with this prefix.
                                          note that this will be overridden if your routes point to function destinations
  -q, --queryParameter strings            query parameters to match on the request. values can be specified using regex strings
  -f, --rest-function-name string         name of the REST function to invoke with this route. use if destination has a REST service spec
      --rest-parameters strings           Parameters for the rest function that are to be read off of incoming request headers. format specified as follows: 'header_name=extractor_string' where header_name is the HTTP2 equivalent header (':path' for HTTP 1 path).
                                          
                                          For example, to extract the variable 'id' from the following request path /users/1, where 1 is the id:
                                          --rest-parameters ':path='/users/{id}'
      --to-route-table                    insert the route into a route table rather than a virtual service
      --upstream-group-name string        name of the upstream group destination for this route
      --upstream-group-namespace string   namespace of the upstream group destination for this route (default "gloo-system")
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
      --use-consul                 use Consul Key-Value storage as the backend for reading and writing config (VirtualServices, Upstreams, and Proxies)
```

### SEE ALSO

* [glooctl add](../glooctl_add)	 - Adds configuration to a top-level Gloo resource

