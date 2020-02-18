package xdsinspection

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"go.uber.org/zap"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_core1 "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/conversion"
	"github.com/gogo/protobuf/proto"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/protoutils"
	"google.golang.org/grpc"
	"sigs.k8s.io/yaml"
)

func GetGlooXdsDump(ctx context.Context, proxyName, namespace string, verboseErrors bool) (*XdsDump, error) {

	xdsPort := strconv.Itoa(int(defaults.GlooXdsPort))
	// If gloo is in MTLS mode
	glooMtlsCheck := exec.Command("kubectl", "get", "configmap", "envoy-sidecar-config", "-n", namespace)
	if err := glooMtlsCheck.Run(); err == nil {
		xdsPort = strconv.Itoa(int(defaults.GlooMtlsModeXdsPort))
	}
	portFwd := exec.Command("kubectl", "port-forward", "-n", namespace,
		"deployment/gloo", xdsPort)
	mergedPortForwardOutput := bytes.NewBuffer([]byte{})
	portFwd.Stdout = mergedPortForwardOutput
	portFwd.Stderr = mergedPortForwardOutput
	if err := portFwd.Start(); err != nil {
		return nil, eris.Wrapf(err, "failed to start port-forward")
	}
	defer func() {
		if portFwd.Process != nil {
			portFwd.Process.Kill()
		}
	}()
	result := make(chan *XdsDump)
	errs := make(chan error)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			time.Sleep(time.Millisecond * 250)
			out, err := getXdsDump(ctx, xdsPort, proxyName, namespace)
			if err != nil {
				errs <- err
				continue
			}
			result <- out
			return
		}
	}()

	timer := time.Tick(time.Second * 5)

	for {
		select {
		case <-ctx.Done():
			return nil, eris.Errorf("cancelled")
		case err := <-errs:
			if verboseErrors {
				contextutils.LoggerFrom(ctx).Errorf("connecting to gloo failed with err %v", err.Error())
			}
		case res := <-result:
			return res, nil
		case <-timer:
			contextutils.LoggerFrom(ctx).Errorf("connecting to gloo failed with err %v",
				zap.Any("cmdErrors", string(mergedPortForwardOutput.Bytes())))
			return nil, eris.Errorf("timed out trying to connect to Envoy admin port")
		}
	}

}

type XdsDump struct {
	Role      string
	Endpoints []v2.ClusterLoadAssignment
	Clusters  []v2.Cluster
	Listeners []v2.Listener
	Routes    []v2.RouteConfiguration
}

func getXdsDump(ctx context.Context, xdsPort, proxyName, proxyNamespace string) (*XdsDump, error) {
	xdsDump := &XdsDump{
		Role: fmt.Sprintf("%v~%v", proxyNamespace, proxyName),
	}
	dr := &v2.DiscoveryRequest{Node: &envoy_api_v2_core1.Node{
		Metadata: &structpb.Struct{
			Fields: map[string]*structpb.Value{"role": {Kind: &structpb.Value_StringValue{StringValue: xdsDump.Role}}}},
	}}

	conn, err := grpc.Dial("localhost:"+xdsPort, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	xdsDump.Endpoints, err = listEndpoints(ctx, dr, conn)
	if err != nil {
		return nil, err
	}

	xdsDump.Clusters, err = listClusters(ctx, dr, conn)
	if err != nil {
		return nil, err
	}

	xdsDump.Listeners, err = listListeners(ctx, dr, conn)
	if err != nil {
		return nil, err
	}

	// dig through hcms in listeners to find routes
	var hcms []envoyhttp.HttpConnectionManager
	for _, l := range xdsDump.Listeners {
		for _, fc := range l.FilterChains {
			for _, filter := range fc.Filters {
				if filter.Name == "envoy.http_connection_manager" {
					var hcm envoyhttp.HttpConnectionManager
					switch config := filter.ConfigType.(type) {
					case *envoylistener.Filter_Config:
						if err := envoyutil.StructToMessage(config.Config, &hcm); err == nil {
							hcms = append(hcms, hcm)
						}
					case *envoylistener.Filter_TypedConfig:
						if err := ptypes.UnmarshalAny(config.TypedConfig, &hcm); err == nil {
							hcms = append(hcms, hcm)
						}
					}
				}
			}
		}
	}

	var routes []string
	for _, hcm := range hcms {
		routes = append(routes, hcm.RouteSpecifier.(*envoyhttp.HttpConnectionManager_Rds).Rds.RouteConfigName)
	}

	xdsDump.Routes, err = listRoutes(ctx, conn, dr, routes)
	if err != nil {
		return nil, err
	}

	return xdsDump, nil
}

func listClusters(ctx context.Context, dr *v2.DiscoveryRequest, conn *grpc.ClientConn) ([]v2.Cluster, error) {

	// clusters
	cdsc := v2.NewClusterDiscoveryServiceClient(conn)
	dresp, err := cdsc.FetchClusters(ctx, dr)
	if err != nil {
		return nil, err
	}
	var clusters []v2.Cluster
	for _, anyCluster := range dresp.Resources {

		var cluster v2.Cluster
		if err := ptypes.UnmarshalAny(anyCluster, &cluster); err != nil {
			return nil, err
		}
		clusters = append(clusters, cluster)
	}
	return clusters, nil
}

func listEndpoints(ctx context.Context, dr *v2.DiscoveryRequest, conn *grpc.ClientConn) ([]v2.ClusterLoadAssignment, error) {
	eds := v2.NewEndpointDiscoveryServiceClient(conn)
	dresp, err := eds.FetchEndpoints(ctx, dr)
	if err != nil {
		return nil, eris.Errorf("endpoints err: %v", err)
	}
	var class []v2.ClusterLoadAssignment

	for _, anyCla := range dresp.Resources {

		var cla v2.ClusterLoadAssignment
		if err := ptypes.UnmarshalAny(anyCla, &cla); err != nil {
			return nil, err
		}
		class = append(class, cla)
	}
	return class, nil
}

func listListeners(ctx context.Context, dr *v2.DiscoveryRequest, conn *grpc.ClientConn) ([]v2.Listener, error) {

	// listeners
	ldsc := v2.NewListenerDiscoveryServiceClient(conn)
	dresp, err := ldsc.FetchListeners(ctx, dr)
	if err != nil {
		return nil, eris.Errorf("listeners err: %v", err)
	}
	var listeners []v2.Listener

	for _, anylistener := range dresp.Resources {
		var listener v2.Listener
		if err := ptypes.UnmarshalAny(anylistener, &listener); err != nil {
			return nil, err
		}
		listeners = append(listeners, listener)
	}
	return listeners, nil
}

func listRoutes(ctx context.Context, conn *grpc.ClientConn, dr *v2.DiscoveryRequest, routenames []string) ([]v2.RouteConfiguration, error) {

	// routes
	ldsc := v2.NewRouteDiscoveryServiceClient(conn)

	dr.ResourceNames = routenames

	dresp, err := ldsc.FetchRoutes(ctx, dr)
	if err != nil {
		return nil, eris.Errorf("routes err: %v", err)
	}
	var routes []v2.RouteConfiguration

	for _, anyRoute := range dresp.Resources {
		var route v2.RouteConfiguration
		if err := ptypes.UnmarshalAny(anyRoute, &route); err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	return routes, nil
}

func (xd *XdsDump) String() string {
	buf := &bytes.Buffer{}
	errString := "unable to parse yaml"

	fmt.Fprintf(buf, "\n\n#role: %v", xd.Role)
	fmt.Fprintf(buf, "\n\n#clusters")
	for _, c := range xd.Clusters {
		yam, err := toYaml(&c)
		if err != nil {
			return errString
		}
		fmt.Fprintf(buf, "\n%s", yam)
	}

	fmt.Fprintf(buf, "\n\n#eds")
	for _, c := range xd.Endpoints {
		yam, err := toYaml(&c)
		if err != nil {
			return errString
		}
		fmt.Fprintf(buf, "\n%s", yam)
	}

	fmt.Fprintf(buf, "\n\n#listeners")
	for _, c := range xd.Listeners {
		yam, err := toYaml(&c)
		if err != nil {
			return errString
		}
		fmt.Fprintf(buf, "\n%s", yam)
	}
	fmt.Fprintf(buf, "\n\n#rds")

	for _, c := range xd.Routes {
		yam, err := toYaml(&c)
		if err != nil {
			return errString
		}
		fmt.Fprintf(buf, "\n%s", yam)
	}

	buf.WriteString("\n")
	return buf.String()
}

func toYaml(pb proto.Message) ([]byte, error) {
	jsn, err := protoutils.MarshalBytes(pb)
	if err != nil {
		return nil, err
	}
	return yaml.JSONToYAML(jsn)
}
