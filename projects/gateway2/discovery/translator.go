package discovery

import (
	"context"
	"fmt"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/solo-io/gloo/projects/gateway2/translator/utils"
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
	clusters, endpoints, warnings := TranslateClusters(ctx, e.cli, false)

	if clusters == nil {
		return ctrl.Result{}, nil
	}

	// TODO: check if endpoints version changed and log warnings if so
	e.inputChannels.UpdateDiscoveryInputs(ctx, xds.DiscoveryInputs{
		Clusters:  clusters,
		Endpoints: endpoints,
		Warnings:  warnings,
	})

	// Send across endpoints and upstreams
	return ctrl.Result{}, nil
}

func TranslateClusters(ctx context.Context, cli client.Client, useVip bool) ([]*clusterv3.Cluster, []*endpointv3.ClusterLoadAssignment, []string) {
	var warnings []string

	svcList := corev1.ServiceList{}
	if err := cli.List(ctx, &svcList); err != nil {
		if client.IgnoreNotFound(err) != nil {
			warnings = append(warnings, fmt.Sprintf("failed to list services: %v", err))
			return nil, nil, warnings
		}
	}

	clusters := []*clusterv3.Cluster{}
	endpoints := []*endpointv3.ClusterLoadAssignment{}

	var sc ServiceConverter
	et := EndpointTranslator{UseVIP: useVip}

	for _, svc := range svcList.Items {
		clusters = append(clusters, sc.ClustersForService(ctx, &svc)...)
		e, w := et.ComputeEndpointsForService(ctx, &svc, cli)
		endpoints = append(endpoints, e...)
		warnings = append(warnings, w...)
	}

	endpoints = fixupClustersAndEndpoints(clusters, endpoints)
	return clusters, endpoints, warnings
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
	hash, err := utils.ProtoFnvHash(nil, cluster)
	if err != nil {
		// should never  happen
		// TODO: log
		return fmt.Sprintf("%s-hashErr", cluster.GetName())
	}

	return fmt.Sprintf("%s-%d", cluster.GetName(), hash)
}
