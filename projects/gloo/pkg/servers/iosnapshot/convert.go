package iosnapshot

import (
	"cmp"
	"slices"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	ratelimitv1alpha1 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/solo/ratelimit"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	graphqlv1beta1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/graphql/v1beta1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	crdv1 "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"
)

// convert all the resources in the input snapshot, excluding Artifacts and Secrets, into Kubernetes format
func snapshotToKubeResources(snap *v1snap.ApiSnapshot) ([]crdv1.Resource, error) {
	resources := []crdv1.Resource{}

	// gloo.solo.io resources
	for _, upstream := range snap.Upstreams {
		kubeUpstream, err := gloov1.UpstreamCrd.KubeResource(upstream)
		if err != nil {
			return nil, err
		}
		resources = append(resources, *kubeUpstream)
	}
	for _, upstreamGroup := range snap.UpstreamGroups {
		kubeUpstreamGroup, err := gloov1.UpstreamGroupCrd.KubeResource(upstreamGroup)
		if err != nil {
			return nil, err
		}
		resources = append(resources, *kubeUpstreamGroup)
	}
	for _, proxy := range snap.Proxies {
		kubeProxy, err := gloov1.ProxyCrd.KubeResource(proxy)
		if err != nil {
			return nil, err
		}
		resources = append(resources, *kubeProxy)
	}
	// Endpoints are only stored in memory and don't have a Kubernetes resource equivalent,
	// so we do custom conversion here to make the format consistent with the other resources
	for _, endpoint := range snap.Endpoints {
		kubeEndpoint, err := convertToKube(endpoint, gloov1.EndpointCrd)
		if err != nil {
			return nil, err
		}
		resources = append(resources, *kubeEndpoint)
	}

	// gateway.solo.io resources
	for _, gw := range snap.Gateways {
		kubeGw, err := gatewayv1.GatewayCrd.KubeResource(gw)
		if err != nil {
			return nil, err
		}
		resources = append(resources, *kubeGw)
	}

	for _, vs := range snap.VirtualServices {
		kubeVs, err := gatewayv1.VirtualServiceCrd.KubeResource(vs)
		if err != nil {
			return nil, err
		}
		resources = append(resources, *kubeVs)
	}
	for _, rt := range snap.RouteTables {
		kubeRt, err := gatewayv1.RouteTableCrd.KubeResource(rt)
		if err != nil {
			return nil, err
		}
		resources = append(resources, *kubeRt)
	}
	for _, vho := range snap.VirtualHostOptions {
		kubeVho, err := gatewayv1.VirtualHostOptionCrd.KubeResource(vho)
		if err != nil {
			return nil, err
		}
		resources = append(resources, *kubeVho)
	}
	for _, rto := range snap.RouteOptions {
		kubeRto, err := gatewayv1.RouteOptionCrd.KubeResource(rto)
		if err != nil {
			return nil, err
		}
		resources = append(resources, *kubeRto)
	}
	for _, hgw := range snap.HttpGateways {
		kubeHgw, err := gatewayv1.MatchableHttpGatewayCrd.KubeResource(hgw)
		if err != nil {
			return nil, err
		}
		resources = append(resources, *kubeHgw)
	}
	for _, tgw := range snap.TcpGateways {
		kubeTgw, err := gatewayv1.MatchableTcpGatewayCrd.KubeResource(tgw)
		if err != nil {
			return nil, err
		}
		resources = append(resources, *kubeTgw)
	}

	// enterprise.gloo.solo.io resources
	for _, ac := range snap.AuthConfigs {
		kubeAc, err := extauthv1.AuthConfigCrd.KubeResource(ac)
		if err != nil {
			return nil, err
		}
		resources = append(resources, *kubeAc)
	}

	// ratelimit.solo.io resources
	for _, rlc := range snap.Ratelimitconfigs {
		kubeRlc, err := ratelimitv1alpha1.RateLimitConfigCrd.KubeResource(rlc)
		if err != nil {
			return nil, err
		}
		resources = append(resources, *kubeRlc)
	}

	// graphql.gloo.solo.io resources
	for _, gqlApi := range snap.GraphqlApis {
		kubeGqlApi, err := graphqlv1beta1.GraphQLApiCrd.KubeResource(gqlApi)
		if err != nil {
			return nil, err
		}
		resources = append(resources, *kubeGqlApi)
	}

	return resources, nil
}

// This converts a solo-kit VersionedResource to a solo-kit Kubernetes resource. It mirrors the
// solo-kit [KubeResource](https://github.com/solo-io/solo-kit/blob/1baf6de465942dc5be44e7f28f0f739dcd0b967b/pkg/api/v1/clients/kube/crd/crd.go#L78)
// func, except that it accepts "fake" resources such as Endpoints, which don't implement
// [InputResource](https://github.com/solo-io/solo-kit/blob/1baf6de465942dc5be44e7f28f0f739dcd0b967b/pkg/api/v1/resources/resource_interface.go#L47)
// (don't have statuses).
func convertToKube(resource resources.VersionedResource, crd crd.Crd) (*crdv1.Resource, error) {
	var spec crdv1.Spec

	data, err := protoutils.MarshalMap(resource)
	if err != nil {
		return nil, err
	}

	delete(data, "metadata")
	spec = data

	return &crdv1.Resource{
		TypeMeta:   crd.TypeMeta(),
		ObjectMeta: kubeutils.ToKubeMetaMaintainNamespace(resource.GetMetadata()),
		Spec:       &spec,
		Status:     crdv1.Status{},
	}, nil
}

func sortResources(resources []crdv1.Resource) {
	slices.SortStableFunc(resources, func(a, b crdv1.Resource) int {
		return cmp.Or(
			cmp.Compare(a.APIVersion, b.APIVersion),
			cmp.Compare(a.Kind, b.Kind),
			cmp.Compare(a.GetNamespace(), b.GetNamespace()),
			cmp.Compare(a.GetName(), b.GetName()),
		)
	})
}
