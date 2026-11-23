# EP-14087: Extensible Kubernetes/KRT API Client

- Issue: [#14087](https://github.com/kgateway-dev/kgateway/issues/14087)

<!-- toc -->

- [Background](#background)
- [Motivation](#motivation)
- [Goals](#goals)
- [Non-Goals](#non-goals)
- [Implementation Details](#implementation-details)
- [Open Questions](#open-questions)

<!-- /toc -->

## Background

kgateway already is built in a modular fashion with individual plugins for implementing specific policy resources.
This plugin system also is accessible to third parties which may extend kgateway without having to maintain a fork,
as per included [example code](/examples/plugin/main.go).

Plugins may want to employ custom Kubernetes resources, defined via CRDs, to provide a rich API for their policies.
This requires "teaching" kgateway's runtime and in particular KRT about these resources.

## Motivation

A central part of KRT is registering types to the runtime via [`kubeclient.Register`](https://pkg.go.dev/istio.io/istio/pkg/config/schema/kubeclient#Register).
This is essentially all that's required to make a custom resource work with KRT and thus as a viable policy resource.

The supplied `ClientGetter` is a centrally provided instance that is shared across plugins. For kgateway, this may be
overridden during setup. A user must take care to implement all interfaces that kgateway's implementation already does.

If a plugin requires access to a custom API client, it must be included in the supplied instance by the plugin end user.
For multiple plugins with different resources, the supplied API client must conform to *all* expected interfaces.
Due to the way Go and its type system works, the user must essentially reimplement the entire client creation process
from scratch.

This is error-prone, as a user might miss a plugin's client interface in their implementation, and cumbersome.
In general, it's not a very pluggable nor composable mechanism with a lot of manual work for integrating different plugins.

Thus, a central solution provided by kgateway is necessary.

## Goals

 * **Pluggable API Client**: Provide an interface for plugins to register their own Kubernetes API clients.
 * **Streamlined Type Registration**: Include custom type registration in the plugin setup procedures.
 * **Plugin Authorship Recommendations**: Extend the "third-party" plugin example with a custom policy resource to showcase best practices.

## Non-Goals

 * Overhauling the plugin system as a whole
 * Installation and management procedures for third-party plugin CRDs

## Implementation Details

An implementation could substitute a "generic client" for the custom client we currently purpose-build for kgateway.

Plugin-/application-specific clients would be registered to this instance utilizing a unique per-client "tag" that
carries type information using Go generics. The generic client could maintain a map of these tags to concrete instances.

A very rough reference for that could be the following:
```go
package apiclient

import (
	"istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube"
	"k8s.io/client-go/rest"
)

// Allow exposing the tag outside the package, but limit creation and implementation to the package itself
// Intended as opaque references only carrying type information
type ClientTag[C any] interface {
	isClientTag() bool
}

type clientTag[C any] struct {
}

func (t *clientTag[C]) isClientTag() bool {
	return true
}

// Container for instances of concrete API clients
type GenericClient struct {
	kube.Client
	// There's no "wildcard" Generic in Go, so we have to use any as the type here.
	// Type safety is enforced by functions.
	clients map[any]any
}

// Retrieves a concrete client from the container based on a tag
// Can't be a method on GenericClient because Go doesn't have generic methods (yet)
func RetrieveClient[C any](client *GenericClient, tag ClientTag[C]) C {
	c, ok := client.clients[tag]
	if !ok {
		panic("unknown client")
	}
	return c.(C)
}

// Builder for a new concrete client
type ClientBuilderFunc[C any] = func(config *rest.Config) (C, error)

type clientInfo struct {
	build    ClientBuilderFunc[any]
	register func()
}

// Container for building out a GenericClient
type ClientBuilder struct {
	clients map[any]clientInfo
}

// RegisterClient adds a new client to the builder based on its builder function
// Also allows registering types and retrieving the concrete client type from the ClientGetter
// Again not a method because no generic methods
func RegisterClient[C any](builder *ClientBuilder, build ClientBuilderFunc[C], register func(getClient func(kubeclient.ClientGetter) C)) ClientTag[C] {
	// Tag identity is based on reference value, not any struct contents
	tag := &clientTag[C]{}
	builder.clients[tag] = clientInfo{
		build: func(config *rest.Config) (any, error) {
			return build(config)
		},
		register: func() {
			register(func(getter kubeclient.ClientGetter) C {
				return RetrieveClient[C](getter.(*GenericClient), tag)
			})
		},
	}
	// Return the tag for later direct uses of RetrieveClient
	return tag
}

func NewBuilder() *ClientBuilder {
	return &ClientBuilder{clients: make(map[any]clientInfo)}
}

// Finalizes the build step and actualizes the client configuration
func (builder *ClientBuilder) Build(restConfig *rest.Config) (*GenericClient, error) {
	restCfg := kube.NewClientConfigForRestConfig(restConfig)
	kubeClient, err := kube.NewClient(restCfg, "")
	if err != nil {
		return nil, err
	}

	clients := make(map[any]any, len(builder.clients))
	for tag, info := range builder.clients {
		c, err := info.build(restConfig)
		if err != nil {
			return nil, err
		}
		clients[tag] = c
		info.register()
	}

	return &GenericClient{
		Client: kubeClient,
		clients: clients,
	}, nil
}
```

Furthermore, the plugin SDK interface could be expanded to allow plugins to specify a builder function for their desired
API client(s), in the vein of `ContributesPolicies`. Type registration could be directly integrated with that.

The setup process would need to be refactored slightly to initialize plugins (or their API clients, at least) earlier.

## Open Questions

<!--
Include any unresolved questions or areas requiring feedback.
-->
