package fds

import (
	"context"
	"net/url"

	"github.com/solo-io/solo-kit/pkg/errors"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/graphql/v1beta1"
	plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
)

type UpstreamMutator func(*v1.Upstream) error

type AdditionalClients struct {
	GraphqlClient v1beta1.GraphQLApiClient
}

/*
upstreams can be obviously functional like AWS λ, fission,...  or an upstream that was already detected and marked as such.
or potentially like static upstreams.
*/
// detectors detect a specific type of functional service
// if they detect the service, they return service info and
// annotations (optional) for the service
// we want to bake sure that detect upstream for aws doesn't do anything
// perhaps we can do just that
type FunctionDiscoveryFactory interface {
	NewFunctionDiscovery(u *v1.Upstream, clients AdditionalClients) UpstreamFunctionDiscovery
}

type Dependencies struct {
	Secrets v1.SecretList
}

type UpstreamFunctionDiscovery interface {
	// if this returns true we can skip DetectUpstreamType and go straight to DetectFunctions
	// if this returns false we should call detect upstream type.
	// if detect Upstream type returns true, we have the type!
	// if it returns false and nil error, it means it was detected to not be of this type -
	// ideally this means that this detector will no longer be used with this upstream. in practice this can be logged/ignored.
	// if it returns false and some error, try again later with back-off/timeout.
	IsFunctional() bool

	// Returns
	// err != nil temporary error. try again
	// err == nil spec == nil. no type detected, don't try again
	// url is never nil
	DetectType(ctx context.Context, url *url.URL) (*plugins.ServiceSpec, error)

	// url maybe nil if it couldn't be resolved
	DetectFunctions(ctx context.Context, url *url.URL, dependencies func() Dependencies, out func(UpstreamMutator) error) error
}

type Resolver interface {
	/*
		TCP if not known
		HTTP/HTTPS if known or perhaps NATS?
	*/
	Resolve(us *v1.Upstream) (*url.URL, error)
}

type Resolvers []Resolver

func (resolvers Resolvers) Resolve(us *v1.Upstream) (*url.URL, error) {
	for _, res := range resolvers {
		u, err := res.Resolve(us)
		if err != nil {
			return nil, err
		}
		if u != nil {
			return u, nil
		}
	}
	return nil, errors.Errorf("no resolver found for upstream %v", us.GetMetadata().GetName())
}

// STEP ONE, for generic upstream, detect
// NEW -> DETECTING -> TYPED()

// flow:
// upstream type: aws
// detector type: swagger (can only be used with upstreams that have a url that's resolvable)
