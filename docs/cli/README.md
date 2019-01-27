# `glooctl`: Gloo's Command-Line Interface

`glooctl` wraps calls to the Gloo API (via interaction with an external key-value store, such as Kubernetes CRDs) to make 
using Gloo easy.

* [Installation](#installation)
  * [Install `glooctl`](#install-the-cli)
  * [Upgrade `glooctl`](#upgrade-the-cli)
  * [`glooctl install kube`](#install-gloo-on-kubernetes)
  * [Uninstall from Kubernetes](#uninstall-gloo-from-kubernetes)
* [Reference](#reference)
* [Upstreams](#upstreams)
  * [`glooctl get upstream`](glooctl_get_upstream.md)
  * [`glooctl create upstream`](glooctl_create_upstream.md)
  * [`glooctl create upstream aws`](glooctl_create_upstream_aws.md)
  * [`glooctl create upstream azure`](glooctl_create_upstream_azure.md)
  * [`glooctl create upstream consul`](glooctl_create_upstream_consul.md)
  * [`glooctl create upstream kube`](glooctl_create_upstream_kube.md)
  * [`glooctl create upstream static`](glooctl_create_upstream_static.md)
  * [`glooctl delete upstream`](glooctl_delete_upstream.md)
* [Virtual Services](#virtual-Services)
  * [`glooctl get virtualservice`](glooctl_get_virtualservice.md)
  * [`glooctl create virtualservice`](glooctl_create_virtualservice.md)
  * [`glooctl delete virtualservice`](glooctl_delete_virtualservice.md)
* [Secrets](#secrets)
  * [`glooctl create secret aws`](glooctl_create_secret_aws.md)
  * [`glooctl create secret tls`](glooctl_create_secret_tls.md)
* [Routes](#routes)
     * [`glooctl add route`](glooctl_add_route.md)
     * [`glooctl remove route`](glooctl_remove_route.md)
     * [`glooctl route sort`](glooctl_route_sort.md)
* [Proxy](#proxy)
  * [`glooctl proxy url`](glooctl_proxy_url.md)
  * [`glooctl proxy logs`](glooctl_proxy_logs.md)
  * [`glooctl proxy stats`](glooctl_proxy_stats.md)


  
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

# Reference

In general glooctl supports interactive mode for commands. to use interactive mode, add the `-i` flag.
See the full command line reference here: [glooctl reference](glooctl.md).

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

Upstreams Reference:

* [glooctl create upstream](glooctl_create_upstream.md)	 - Create Upstreams
* [glooctl get upstream](glooctl_create_upstream.md)	 - List Upstreams
* [glooctl delete upstream](glooctl_delete_upstream.md)	 - Delete Upstreams



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

Virtual Services Reference:

* [glooctl create virtualservice](glooctl_create_virtualservice.md)	 - Create Virtual Services
* [glooctl get virtualservice](glooctl_create_virtualservice.md)	 - List Virtual Services
* [glooctl get virtualservice](glooctl_delete_virtualservice.md)	 - Delete Virtual Services


---


# Secrets

Secrets contain private data (access keys, passwords, etc.) that 
should not be stored in configuration objects like Upstreams and Virtual Services.

Instead, Secrets are their own kind of object (usually backed by Kubernetes Secrets, with Vault integration supported).

`glooctl` provides some convenience commands for creating secrets with ease.


Secrets Reference:

* [glooctl create secret](glooctl_create_secret.md)	 - Create Secrets


---


# Routes


Routes Reference:

* [glooctl add route](glooctl_add_route.md)	 - Add Routes
* [glooctl remove route](glooctl_remove_route.md)	 - Remove Routes
* [glooctl route sort](glooctl_route_sort.md)	 - Sort Routes


---


# Proxy

these commands can be used to interact directly with the Proxies Gloo is managing. They are useful for interacting with and debugging the proxies (Envoy instances) directly.

Proxy Reference:

* [glooctl proxy url](glooctl_proxy_url.md)	 - View the HTTP URL of a Proxy
* [glooctl proxy dump](glooctl_proxy_dump.md)	 - Dump Envoy config
* [glooctl proxy logs](glooctl_proxy_logs.md)	 - Dump Envoy logs
* [glooctl proxy stats](glooctl_proxy_stats.md)	 - Dump Envoy stats
