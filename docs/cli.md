# `glooctl`: Gloo's Command-Line Interface

`glooctl` wraps calls to the Gloo API (via interaction with an external key-value store, such as Kubernetes CRDs) to make 
using Gloo easy.

* [Installation](cli.md#Installation)
      * [Install `glooctl`](cli.md#install-the-cli)
      * [Upgrade `glooctl`](cli.md#upgrade-the-cli)
      * [`glooctl install kube`](cli.md#install-gloo-on-kubernetes)
      * [Uninstall from Kubernetes](cli.md#uninstall-gloo-from-kubernetes)
* [Upstreams](cli.md#Upstreams)
      * [`glooctl get upstream`](cli.md#list-upstreams)
      * [`glooctl create upstream`](cli.md#create-upstreams)
      * [`glooctl create upstream aws`](cli.md#create-aws-upstreams)
      * [`glooctl create upstream azure`](cli.md#create-azure-upstreams)
      * [`glooctl create upstream consul`](cli.md#create-consul-upstreams)
      * [`glooctl create upstream kube`](cli.md#create-kubernetes-upstreams)
      * [`glooctl create upstream static`](cli.md#create-static-upstreams)
      * [`glooctl delete upstream`](cli.md#delete-upstreams)
* [Virtual Services](cli.md#Virtual-Services)
      * [`glooctl get virtualservice`](cli.md#list-virtual-services)
      * [`glooctl create virtualservice`](cli.md#create-virtual-services)
      * [`glooctl delete virtualservice`](cli.md#delete-virtual-services)
* [Secrets](cli.md#Secrets)
      * [`glooctl create secret aws`](cli.md#create-aws-credentials-secret)
      * [`glooctl create secret tls`](cli.md#create-tls-secret)
* [Routes](cli.md#Routes)
     * [`glooctl add route`](cli.md#add-routes)
* [Proxy](cli.md#Proxy)
      * [`glooctl proxy url`](cli.md#proxy-url)
      * [`glooctl proxy config`](cli.md#proxy-config)
      * [`glooctl proxy logs`](cli.md#proxy-logs)
      * [`glooctl proxy stats`](cli.md#proxy-stats)


  
---

# Installation

---

#### Install the CLI

Starting in Gloo 0.5.0, `glooctl` is released with Gloo and can be downloaded from 
the [releases page on github](https://github.com/solo-io/gloo/releases). Download 
`glooctl` and place it somewhere in your `PATH`. 

---

#### Upgrade the CLI

Once you have installed `glooctl`, you can use it to upgrade `glooctl` (and your installation) without 
any other steps.

Usage:
```bash
  glooctl upgrade [flags]
```

Aliases:
```bash
  upgrade, ug
```

Flags:
```bash
  -h, --help             help for upgrade
      --path string      Desired path for your upgraded glooctl binary. Defaults to the location of your currently executing binary.
      --release string   Which glooctl release to download. Specify a git tag corresponding to the desired version of glooctl. (default "latest")
```

#### Install Gloo on Kubernetes

Gloo can be easily installed to kubernetes with the command

```
glooctl install kube
```

This will deploy Gloo and all of its components to the `gloo-system` namespace.

If you are using the Enterprise version of Gloo, you'll need to install
Gloo by passing authorized Docker Hub credentials to the CLI like so:

```bash
glooctl install kube \
    --docker-email=YOUR_EMAIL \
    --docker-username=YOUR_USERNAME \
    --docker-password=YOUR_PASSWORD 
```

To verify the installation succeeded:
```bash
kubectl get pod -n gloo-system

NAME                             READY     STATUS    RESTARTS   AGE
discovery-77467d765f-7jbtx       1/1       Running   0          1d
gateway-676d756695-nvkj2         1/1       Running   0          1d
gateway-proxy-596c4bd9f7-2vn47   1/1       Running   0          1d
gloo-665d768998-ph9sj            1/1       Running   0          1d
rate-limit-7ffc67c798-8cvv2      1/1       Running   0          1d
redis-66db7fdf56-d9ntd           1/1       Running   0          1d

```
---

#### Uninstall Gloo from Kubernetes

To uninstall Gloo, simply delete the `gloo-system` namespace with `kubectl`:

```bash
kubectl delete namespace gloo-system
```
---


# Upstreams

Upstreams represent destinations for routing with Gloo. Upstreams 
are typically associated with a some information that resolves to a set 
of network addresses for a service, or account information for a supported 
cloud provider. 

Some upstreams will be automatically discovered by Gloo's
Discovery service. 

Upstreams can describe details about gRPC and RESTful applications 
for fine-grained routing on the function level with Gloo.

---
#### List Upstreams

```bash
glooctl get upstream
```

usage: `glooctl get upstream [NAME] [--namespace=namespace] [-o FORMAT] [-o FORMAT]`

Aliases:
  `upstream, u, us, upstreams`

Flags:
```
  -h, --help               help for upstream
  -i, --interactive        use interactive mode
      --name string        name of the upstream to read. if empty, will return a list
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
  -o, --output string      output format: (yaml, json...)
```

---
#### Create Upstreams

```bash
glooctl create upstream
```

`glooctl create upstream` defaults to interactive mode. To create an upstream statically, use one of the 
`glooctl create upstream` subcommands.

Aliases:
```bash
  upstream, us, upstream, upstreams
```

Available Commands:
```bash
  aws         Create an Aws Upstream
  azure       Create an Azure Upstream
  consul      Create a Consul Upstream
  kube        Create a Kubernetes Upstream
  static      Create a Static Upstream
```

Flags:
```bash
  -h, --help               help for upstream
      --name string        name of the resource to read or write
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
```

Global Flags:
```bash
  -o, --output string   output format: (yaml, json, table)
```

Use `glooctl create upstream [command] --help` for more information about a command.


---
#### Create Aws Upstreams

```bash
glooctl create upstream aws
```

Use `glooctl create upstream aws -i` for interactive mode.

AWS Upstreams represent a set of AWS Lambda Functions for a Region that can be routed to with Gloo. AWS Upstreams require a valid set of AWS Credentials to be provided. These should be uploaded to Gloo using `glooctl create secret aws`

Usage:

```bash
  glooctl create upstream aws [flags]
```

Flags:
```bash
      --aws-region string                                       region for AWS services this upstream utilize (default "us-east-1")
      --aws-secret-name glooctl create secret aws --help        name of a secret containing AWS credentials created with glooctl. See glooctl create secret aws --help for help creating secrets
      --aws-secret-namespace glooctl create secret aws --help   namespace where the AWS secret lives. See glooctl create secret aws --help for help creating secrets (default "gloo-system")
  -h, --help                                                    help for aws
```

Global Flags:
```bash
  -i, --interactive        use interactive mode
      --name string        name of the resource to read or write
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
  -o, --output string      output format: (yaml, json, table)
```

---
#### Create Azure Upstreams

```bash
glooctl create upstream azure
```

Use `glooctl create upstream azure -i` for interactive mode.

Azure Upstreams represent a set of Azure Functions for a Function App that can be routed to with Gloo. Azure Upstreams require a valid set of Azure Credentials to be provided. These should be uploaded to Gloo using `glooctl create secret azure`

Usage:
```bash
  glooctl create upstream azure [flags]
```

Flags:
```bash
      --azure-app-name string                                       name of the Azure Functions app to associate with this upstream
      --azure-secret-name glooctl create secret azure --help        name of a secret containing Azure credentials created with glooctl. See glooctl create secret azure --help for help creating secrets (default "gloo-system")
      --azure-secret-namespace glooctl create secret azure --help   namespace where the Azure secret lives. See glooctl create secret azure --help for help creating secrets (default "gloo-system")
  -h, --help                                                        help for azure
```

Global Flags:
```bash
  -i, --interactive        use interactive mode
      --name string        name of the resource to read or write
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
  -o, --output string      output format: (yaml, json, table)
```

---
#### Create Consul Upstreams

```bash
glooctl create upstream consul
```

Use `glooctl create upstream consul -i` for interactive mode.

Consul Upstreams represent a collection of endpoints for Services registered with Consul. Typically, Gloo will automatically discover these upstreams, meaning you don't have to create them. However, if upstream discovery in Gloo is disabled, or ACL permissions have not been granted to Gloo to read from the registry, Consul services can be added to Gloo manually via the CLI.

Usage:
```bash
  glooctl create upstream consul [flags]
```

Flags:
```bash
      --consul-service string         name of the service in the consul registry
      --consul-service-tags strings   tags for choosing a subset of the service in the consul registry
  -h, --help                          help for consul
      --service-spec-type string      if set, Gloo supports additional routing features to upstreams with a service spec. The service spec defines a set of features
```

Global Flags:
```bash
  -i, --interactive        use interactive mode
      --name string        name of the resource to read or write
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
  -o, --output string      output format: (yaml, json, table)
```


---
#### Create Kubernetes Upstreams

```bash
glooctl create upstream kube
```

Use `glooctl create upstream kube -i` for interactive mode.


Kubernetes Upstreams represent a collection of endpoints for Services registered with Kubernetes. Typically, Gloo will automatically discover these upstreams, meaning you don't have to create them. However, if upstream discovery in Gloo is disabled, or RBAC permissions have not been granted to Gloo to read from the registry, Kubernetes services can be added to Gloo manually via the CLI.

Usage:
```bash
  glooctl create upstream kube [flags]
```

Flags:
```bash
  -h, --help                            help for kube
      --kube-service string             name of the kubernetes service
      --kube-service-labels strings     labels to use for customized selection of pods for this upstream. can be used to select subsets of pods for a service e.g. for blue-green deployment
      --kube-service-namespace string   namespace where the kubernetes service lives (default "defaukt")
      --kube-service-port uint32        the port were the service is listening. for services listenin on multiple ports, create an upstream for each port. (default 80)
      --service-spec-type string        if set, Gloo supports additional routing features to upstreams with a service spec. The service spec defines a set of features
```

Global Flags:
```bash
  -i, --interactive        use interactive mode
      --name string        name of the resource to read or write
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
  -o, --output string      output format: (yaml, json, table)
```



---
#### Create Static Upstreams

```bash
glooctl create upstream static
```

Use `glooctl create upstream static -i` for interactive mode.

Static upstreams are intended to connect Gloo to upstreams to services (often external or 3rd-party) running at a fixed IP address or hostname. Static upstreams require you to manually specify the hosts associated with a static upstream. Requests routed to a static upstream will be round-robin load balanced across each host.

Usage:
```bash
  glooctl create upstream static [flags]
```

Flags:
```bash
  -h, --help                       help for static
      --service-spec-type string   if set, Gloo supports additional routing features to upstreams with a service spec. The service spec defines a set of features 
      --static-hosts strings       list of hosts for the static upstream. these are hostnames or ips provided in the format IP:PORT or HOSTNAME:PORT. if :PORT is missing, it will default to :80
      --static-outbound-tls        connections Gloo manages to this cluster will attempt to use TLS for outbound connections. Gloo will automatically set this to true for port 443
```

Global Flags:
```bash
  -i, --interactive        use interactive mode
      --name string        name of the resource to read or write
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
  -o, --output string      output format: (yaml, json, table)
```


---
#### Delete Upstreams

```bash
glooctl delete upstream NAME
```

Deletes an upstream from Gloo's catalog. If the upstream was created by 
Discovery, this will not prevent the upstream from being re-generated.

Aliases:
```bash
  upstream, u, us, upstreams
```

Flags:
```bash
  -h, --help   help for upstream
```

Global Flags:
```bash
  -i, --interactive        use interactive mode
      --name string        name of the resource to read or write
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
```






---


# Virtual Services

Virtual Services represent a logical service presented to the 
outside world on the gateway level. When clients communicate with the 
Gateway managed by Gloo, they specify the virtual service they 
wish to interact with by the domain name in their requests. This 
maps to the `Host` header in HTTP1, and the `:authority` header in 
HTTP2.

Virtual services route client requests to upstreams, optionally providing 
additional transport features such as retries, rate limiting, request transformation, 
and more. 

Virtual services are defined by an ordered list of routes 
and the set of domains for which they apply. Domains cannot 
overlap between virtual services or Gloo will report an errored 
configuration.

---
#### List Virtual Services


```bash
`glooctl get virtualservice [NAME] [--namespace=namespace] [-o FORMAT]`
```

Aliases:
```bash
   virtualservice, vs, virtualservices
```

Available Commands:
```bash
  route       get a list of routes for a given virtual service
```

Flags:
```bash
  -h, --help   help for virtualservice
```

Global Flags:
```bash
  -i, --interactive        use interactive mode
      --name string        name of the resource to read or write
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
  -o, --output string      output format: (yaml, json, table)
```


---
#### Create Virtual Services

Create a virtual service to start adding routes. You can choose to skip 
this step and go directly to adding routes. If you do, a basic virtual service 
will be created for you.

```bash
glooctl create virtualservice
```

Usage:
```bash
  glooctl create virtualservice [flags]
```

Aliases:
```bash
  virtualservice, vs, virtualservice, virtualservices
```

Flags:
```bash
      --domains strings               comma seperated list of domains
      --enable-rate-limiting          enable rate limiting features for this virtual service
  -h, --help                          help for virtualservice
      --name string                   name of the resource to read or write
  -n, --namespace string              namespace for reading or writing resources (default "gloo-system")
      --rate-limit-requests uint32    requests per unit of time (default 100)
      --rate-limit-time-unit string   unit of time over which to apply the rate limit (default "MINUTE")
```

Global Flags:
```bash
  -i, --interactive     use interactive mode
  -o, --output string   output format: (yaml, json, table)
```


---
#### Delete Virtual Services

This will delete all routes that have been added to the virtual service.

usage: `glooctl delete virtualservice [NAME] [--namespace=namespace]`

Usage:
```bash
  glooctl delete virtualservice [flags]
```

Aliases:
```bash
  virtualservice, v, vs, virtualservices
```

Flags:
```bash
  -h, --help   help for virtualservice
```

Global Flags:
```bash
  -i, --interactive        use interactive mode
      --name string        name of the resource to read or write
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
```



---


# Secrets

Secrets contain private data (access keys, passwords, etc.) that 
should not be stored in configuration objects like Upstreams and Virtual Services.

Instead, Secrets are their own kind of object (usually backed by Kubernetes Secrets, with Vault integration supported).

`glooctl` provides some convenience commands for creating secrets with ease.

---
#### Create AWS Credentials Secret

Create an AWS secret with the given name. If no flags are provided, 
the default profile will be read from `~/.aws/credentials` and used 
to generate the secret for Gloo to read.

Usage:
```bash
  glooctl create secret aws [flags]
```

Flags:
```bash
      --access-key string   aws access key
  -h, --help                help for aws
      --name string         name of the resource to read or write
  -n, --namespace string    namespace for reading or writing resources (default "gloo-system")
      --secret-key string   aws secret key
```

Global Flags:
```bash
  -i, --interactive     use interactive mode
  -o, --output string   output format: (yaml, json, table)
```

---
#### Create TLS Secret

The TLS secret contains the root certificate, private key, and cert chain necessary 
for encrypting TCP traffic. TLS secrets must be provided to Gloo to configure Virtual Services 
to use SSL.

Usage:
```bash
  glooctl create secret tls [flags]
```

Flags:
```bash
      --certchain string    filename of certchain for secret
  -h, --help                help for tls
      --privatekey string   filename of privatekey for secret
      --rootca string       filename of rootca for secret
```

Global Flags:
```bash
  -i, --interactive     use interactive mode
  -o, --output string   output format: (yaml, json, table)
```




---


# Routes

Add HTTP routes to a virtualservice using the `glooctl add route` command.

Adding routes in *interactive* mode is recommended for beginners.
Simply run `glooctl add route -i` to use interactive mode.

---
#### Add Routes

Usage:
```bash
  glooctl add route [flags]
```

Aliases:
```bash
  route, r, routes
```

Flags:
```bash
  -a, --aws-function-name string    logical name of the AWS lambda to invoke with this route. use if destination is an AWS upstream
      --aws-unescape                unescape JSON returned by this lambda function (useful if the response is not intended to be JSON formatted, e.g. in the case of static content (images, HTML, etc.) being served by Lambda
  -u, --dest-name string            name of the destination upstream for this route
  -s, --dest-namespace string       namespace of the destination upstream for this route (default "gloo-system")
  -t, --dest-type string            type of the destination being routed to. this is optional depending on the upstream type. required if you wish to invoke a function, e.g. invoke a Lambda function or gRPC method.
  -d, --header strings              headers to match on the request. values can be specified using regex strings
  -h, --help                        help for route
  -x, --index uint32                index in the virtual service route list where to insert this route. routes after it will be shifted back one
  -m, --method strings              the HTTP methods (GET, POST, etc.) to match on the request. if empty, all methods will match
  -o, --output string               output format: (yaml, json, table)
  -e, --path-exact string           regex matcher for route. note: only one of path-exact, path-regex, or path-prefix should be set
  -p, --path-prefix string          path prefix to match route
  -r, --path-regex string           exact path to match route
  -f, --rest-function-name string   name of the REST function to invoke with this route. use if destination has a REST service spec
      --rest-parameters strings     Parameters for the rest function that are to be read off of incoming request headers. format specified as follows: 'header_name=extractor_string' where header_name is the HTTP2 equivalent header (':path' for HTTP 1 path).

                                    For example, to extract the variable 'id' from the following request path /users/1, where 1 is the id:
                                    --rest-parameters ':path='/users/{id}'
```

Global Flags:
```bash
  -i, --interactive        use interactive mode
      --name string        name of the resource to read or write
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
```





---


# Proxy

these commands can be used to interact directly with the Proxies Gloo is managing. They are useful for interacting with and debugging the proxies (Envoy instances) directly.

---
#### Proxy URL


Use this command to view the HTTP URL of a Proxy reachable from outside the cluster. You can connect to this address from a host on the same network (such as the host machine, in the case of minikube/minishift).

Usage:
```bash
  glooctl proxy url [flags]
```

Flags:
```bash
  -l, --local-cluster      use when the target kubernetes cluster is running locally, e.g. in minikube or minishift. this will default to true if LoadBalanced services are not assigned external IPs by your cluster
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
  -i, --interactive   use interactive mode
  -p, --name string   the name of the proxy service/deployment to use (default "gateway-proxy")
      --port string   the name of the service port to connect to (default "http")
```

---
#### Proxy Config

dump Envoy config from one of the proxy instances

Usage:
  glooctl proxy dump [flags]

Flags:
  -h, --help               help for dump
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")


---
#### Proxy Logs

dump Envoy logs from one of the proxy instances

Usage:
```bash
  glooctl proxy logs [flags]
```

Flags:
```bash
  -h, --help               help for stats
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
```


---
#### Proxy Stats

dump Envoy stats from one of the proxy instances

Usage:
```bash
  glooctl proxy stats [flags]
```

Flags:
```bash
  -h, --help               help for logs
  -n, --namespace string   namespace for reading or writing resources (default "gloo-system")
```
