package gateway

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_core1 "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/go-utils/errors"
	"github.com/solo-io/go-utils/protoutils"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"sigs.k8s.io/yaml"
)

func servedConfigCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "served-config",
		Short: "dump Envoy config being served by the Gloo xDS server",
		RunE: func(cmd *cobra.Command, args []string) error {
			servedConfig, err := printGlooXdsDump(opts)
			if err != nil {
				return err
			}
			fmt.Printf("%v", servedConfig)
			return nil
		},
	}
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func printGlooXdsDump(opts *options.Options) (string, error) {
	xdsPort := strconv.Itoa(int(defaults.GlooXdsPort))
	portFwd := exec.Command("kubectl", "port-forward", "-n", opts.Metadata.Namespace,
		"deployment/gloo", xdsPort)
	portFwd.Stdout = os.Stderr
	portFwd.Stderr = os.Stderr
	if err := portFwd.Start(); err != nil {
		return "", errors.Wrapf(err, "failed to start port-forward")
	}
	defer func() {
		if portFwd.Process != nil {
			portFwd.Process.Kill()
		}
	}()
	result := make(chan string)
	errs := make(chan error)
	go func() {
		for {
			select {
			case <-opts.Top.Ctx.Done():
				return
			default:
			}
			out, err := getXdsDump(opts.Top.Ctx, xdsPort, opts.Proxy.Name, opts.Metadata.Namespace)
			if err != nil {
				errs <- err
				time.Sleep(time.Millisecond * 250)
				continue
			}
			result <- out.String()
			return
		}
	}()

	timer := time.Tick(time.Second * 5)

	for {
		select {
		case <-opts.Top.Ctx.Done():
			return "", errors.Errorf("cancelled")
		case err := <-errs:
			log.Printf("connecting to gloo failed with err %v", err.Error())
		case res := <-result:
			return res, nil
		case <-timer:
			return "", errors.Errorf("timed out trying to connect to Envoy admin port")
		}
	}

}

func getXdsDump(ctx context.Context, xdsPort, proxyName, proxyNamespace string) (fmt.Stringer, error) {
	buf := &bytes.Buffer{}
	role := fmt.Sprintf("%v~%v", proxyNamespace, proxyName)
	dr := &v2.DiscoveryRequest{Node: &envoy_api_v2_core1.Node{
		Metadata: &types.Struct{
			Fields: map[string]*types.Value{"role": {Kind: &types.Value_StringValue{StringValue: role}}}},
	}}

	conn, err := grpc.Dial("localhost:"+xdsPort, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	eds, err := listEndpoints(ctx, dr, conn)
	if err != nil {
		return nil, err
	}

	clusters, err := listClusters(ctx, dr, conn)
	if err != nil {
		return nil, err
	}

	listeners, err := listListeners(ctx, dr, conn)
	if err != nil {
		return nil, err
	}

	// dig through hcms in listeners to find routes
	var hcms []envoyhttp.HttpConnectionManager
	for _, l := range listeners {
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
						if err := types.UnmarshalAny(config.TypedConfig, &hcm); err == nil {
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

	rds, err := listRoutes(ctx, conn, dr, routes)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(buf, "\n\n#clusters")
	for _, c := range clusters {
		yam, err := toYaml(&c)
		if err != nil {
			return nil, err
		}

		fmt.Fprintf(buf, "\n%s", yam)
	}

	fmt.Fprintf(buf, "\n\n#eds")
	for _, c := range eds {
		yam, err := toYaml(&c)
		if err != nil {
			return nil, err
		}

		fmt.Fprintf(buf, "\n%s", yam)
	}

	fmt.Fprintf(buf, "\n\n#listeners")
	for _, c := range listeners {
		yam, err := toYaml(&c)
		if err != nil {
			return nil, err
		}

		fmt.Fprintf(buf, "\n%s", yam)
	}
	fmt.Fprintf(buf, "\n\n#rds")
	for _, c := range rds {
		yam, err := toYaml(&c)
		if err != nil {
			return nil, err
		}

		fmt.Fprintf(buf, "\n%s", yam)
	}
	buf.WriteString("\n")
	return buf, nil
}

func toYaml(pb proto.Message) ([]byte, error) {
	jsn, err := protoutils.MarshalBytes(pb)
	if err != nil {
		return nil, err
	}
	return yaml.JSONToYAML(jsn)
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
		if err := cluster.Unmarshal(anyCluster.Value); err != nil {
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
		log.Fatalf("endpoints err: %v", err)
	}
	var class []v2.ClusterLoadAssignment

	for _, anyCla := range dresp.Resources {

		var cla v2.ClusterLoadAssignment
		if err := cla.Unmarshal(anyCla.Value); err != nil {
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
		log.Fatalf("listeners err: %v", err)
	}
	var listeners []v2.Listener

	for _, anylistener := range dresp.Resources {
		var listener v2.Listener
		if err := listener.Unmarshal(anylistener.Value); err != nil {
			return nil, err
		}
		listeners = append(listeners, listener)
	}
	return listeners, nil
}

func listRoutes(ctx context.Context, conn *grpc.ClientConn, dr *v2.DiscoveryRequest, routenames []string) ([]v2.RouteConfiguration, error) {

	// listeners
	ldsc := v2.NewRouteDiscoveryServiceClient(conn)

	dr.ResourceNames = routenames

	dresp, err := ldsc.FetchRoutes(ctx, dr)
	if err != nil {
		log.Fatalf("routes err: %v", err)
	}
	var routes []v2.RouteConfiguration

	for _, anyRoute := range dresp.Resources {
		var route v2.RouteConfiguration
		if err := route.Unmarshal(anyRoute.Value); err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	return routes, nil
}
