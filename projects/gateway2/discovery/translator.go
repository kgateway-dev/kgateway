package discovery

import (
	"context"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/solo-io/gloo/projects/gateway2/xds"
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

	return nil
}
