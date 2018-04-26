package main

import (
	"context"
	"log"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_core1 "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"

	"github.com/k0kubun/pp"
	"google.golang.org/grpc"
)

var dr v2.DiscoveryRequest

func init() {
	dr.Node = new(envoy_api_v2_core1.Node)
	dr.Node.Id = "oneid"
	dr.Node.Cluster = "ingress"
}

func main() {
	conn, err := grpc.Dial("localhost:8081", grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	listClusters(ctx, conn)
	listerners := listListeners(ctx, conn)
	var hcms []envoyhttp.HttpConnectionManager
	for _, l := range listerners {
		for _, fc := range l.FilterChains {
			for _, filter := range fc.Filters {
				if filter.Name == "envoy.http_connection_manager" {
					var hcm envoyhttp.HttpConnectionManager
					if err := envoyutil.StructToMessage(filter.Config, &hcm); err == nil {
						hcms = append(hcms, hcm)
					}
				}
			}
		}
	}

	var routes []string
	for _, hcm := range hcms {
		routes = append(routes, hcm.RouteSpecifier.(*envoyhttp.HttpConnectionManager_Rds).Rds.RouteConfigName)
	}

	listRoutes(ctx, conn, routes)

	listendpoints(ctx, conn)
}

func listClusters(ctx context.Context, conn *grpc.ClientConn) []v2.Cluster {

	// clusters
	cdsc := v2.NewClusterDiscoveryServiceClient(conn)
	dresp, err := cdsc.FetchClusters(ctx, &dr)
	if err != nil {
		log.Fatal(err)
	}
	var clusters []v2.Cluster
	for _, anyCluster := range dresp.Resources {

		var cluster v2.Cluster
		cluster.Unmarshal(anyCluster.Value)
		clusters = append(clusters, cluster)
		pp.Printf("%v\n", cluster)
	}
	return clusters
}

func listendpoints(ctx context.Context, conn *grpc.ClientConn) []v2.ClusterLoadAssignment {
	eds := v2.NewEndpointDiscoveryServiceClient(conn)
	dresp, err := eds.FetchEndpoints(ctx, &dr)
	if err != nil {
		log.Fatal(err)
	}
	var clas []v2.ClusterLoadAssignment

	for _, anyCla := range dresp.Resources {

		var cla v2.ClusterLoadAssignment
		cla.Unmarshal(anyCla.Value)
		pp.Printf("%v\n", cla)
	}
	return clas
}

func listListeners(ctx context.Context, conn *grpc.ClientConn) []v2.Listener {

	// listeners
	ldsc := v2.NewListenerDiscoveryServiceClient(conn)
	dresp, err := ldsc.FetchListeners(ctx, &dr)
	if err != nil {
		log.Fatal(err)
	}
	var listeners []v2.Listener

	for _, anylistener := range dresp.Resources {

		var listener v2.Listener
		listener.Unmarshal(anylistener.Value)
		pp.Printf("%v\n", listener)
	}
	return listeners
}

func listRoutes(ctx context.Context, conn *grpc.ClientConn, routenames []string) []v2.RouteConfiguration {

	// listeners
	ldsc := v2.NewRouteDiscoveryServiceClient(conn)

	drr := dr

	drr.ResourceNames = routenames

	dresp, err := ldsc.FetchRoutes(ctx, &drr)
	if err != nil {
		log.Fatal(err)
	}
	var routes []v2.RouteConfiguration

	for _, anyRoute := range dresp.Resources {

		var route v2.RouteConfiguration
		route.Unmarshal(anyRoute.Value)
		pp.Printf("%v\n", route)
	}
	return routes
}
