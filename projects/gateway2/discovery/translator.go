package discovery

import (
	"context"
	"fmt"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/solo-io/gloo/projects/gateway2/xds"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	_ Translator = new(edgeLegacyTranslator)
)

// Translator is the interface that discovery components must implement to be used by the discovery controller
// They are responsible for translating Kubernetes resources into the intermediary representation
type Translator interface {
	ReconcilePod(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
	ReconcileService(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
	ReconcileEndpoints(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
}

func NewTranslator(cli client.Client, inputChannels *xds.XdsInputChannels) Translator {
	return &edgeLegacyTranslator{
		cli:           cli,
		inputChannels: inputChannels,
		// snapshot: &OutputSnapshot{},
	}
}

// edgeLegacyTranslator is an implementation of discovery translation that relies on the Gloo Edge
// EDS and UDS implementations. These operate as a batch and are known to not be performant.
type edgeLegacyTranslator struct {
	cli           client.Client
	inputChannels *xds.XdsInputChannels
}

func (e *edgeLegacyTranslator) ReconcilePod(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return e.reconcileAll(ctx)
}

func (e *edgeLegacyTranslator) ReconcileService(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return e.reconcileAll(ctx)
}

func (e *edgeLegacyTranslator) ReconcileEndpoints(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return e.reconcileAll(ctx)
}

func (e *edgeLegacyTranslator) reconcileAll(ctx context.Context) (ctrl.Result, error) {
	// TODO:
	// 1. List resources (services, endpoints, pods) using dynamic client
	// 2. Feed resources into EDS/UDS methods from Gloo Edge, which produce Gloo Endpoints and Upstreams
	// 3. Store the snapshot
	// 4. Signal that a new snapshot is available

	svcList := corev1.ServiceList{}
	if err := e.cli.List(ctx, &svcList); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var clusters []*clusterv3.Cluster
	var endpoints []*endpointv3.ClusterLoadAssignment
	var warnings []string

	var sc ServiceConverter

	for _, svc := range svcList.Items {
		clusters = append(clusters, sc.ClustersForService(ctx, &svc)...)
		e, w := ComputeEndpointsForService(ctx, &svc, e.cli)
		endpoints = append(endpoints, e...)
		warnings = append(warnings, w...)
	}

	endpoints = fixupClustersAndEndpoints(clusters, endpoints)

	// TODO: check if endpoints version changed and log warnings if so
	e.inputChannels.UpdateDiscoveryInputs(ctx, xds.DiscoveryInputs{
		Clusters:  clusters,
		Endpoints: endpoints,
	})

	// Send across endpoints and upstreams
	return ctrl.Result{}, nil
}

func fixupClustersAndEndpoints(

	clusters []*clusterv3.Cluster,
	endpoints []*endpointv3.ClusterLoadAssignment,
) []*endpointv3.ClusterLoadAssignment {

	endpointMap := make(map[string][]*endpointv3.ClusterLoadAssignment, len(endpoints))
	for _, ep := range endpoints {
		if _, ok := endpointMap[ep.GetClusterName()]; !ok {
			endpointMap[ep.GetClusterName()] = []*endpointv3.ClusterLoadAssignment{ep}
		} else {
			// TODO: should check why has duplicated upstream
			endpointMap[ep.GetClusterName()] = append(endpointMap[ep.GetClusterName()], ep)
		}
	}

ClusterLoop:
	for _, c := range clusters {
		if c.GetType() != clusterv3.Cluster_EDS {
			continue
		}
		endpointClusterName := getEndpointClusterName(c)
		// Workaround for envoy bug: https://github.com/envoyproxy/envoy/issues/13009
		// Change the cluster eds config, forcing envoy to re-request latest EDS config
		c.GetEdsClusterConfig().ServiceName = endpointClusterName
		if eList, ok := endpointMap[c.GetName()]; ok {
			for _, ep := range eList {
				// the endpoint ClusterName needs to match the cluster's EdsClusterConfig ServiceName
				ep.ClusterName = endpointClusterName
			}
			continue ClusterLoop
		}
		// we don't have endpoints, set empty endpoints
		emptyEndpointList := &endpointv3.ClusterLoadAssignment{
			ClusterName: endpointClusterName,
		}
		// make sure to call EndpointPlugin with empty endpoint
		// if _, ok := upstreamMap[c.GetName()]; ok {
		// 	for _, plugin := range t.pluginRegistry.GetEndpointPlugins() {
		// 		if err := plugin.ProcessEndpoints(params, upstream, emptyEndpointList); err != nil {
		// 			reports.AddError(upstream, err)
		// 		}
		// 	}
		// }
		if _, ok := endpointMap[emptyEndpointList.GetClusterName()]; !ok {
			endpointMap[emptyEndpointList.GetClusterName()] = []*endpointv3.ClusterLoadAssignment{emptyEndpointList}
		} else {
			endpointMap[emptyEndpointList.GetClusterName()] = append(endpointMap[emptyEndpointList.GetClusterName()], emptyEndpointList)
		}
		endpoints = append(endpoints, emptyEndpointList)
	}

	return endpoints
}

func getEndpointClusterName(cluster *clusterv3.Cluster) string {
	hash, err := translator.EnvoyCacheResourcesListToFnvHash([]envoycache.Resource{resource.NewEnvoyResource(cluster)})
	if err != nil {
		// should never  happen
		// TODO: log
	}

	return fmt.Sprintf("%s-%d", cluster.GetName(), hash)
}
