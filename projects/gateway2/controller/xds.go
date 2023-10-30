package controller

import (
	"context"
	"fmt"
	"net"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/go-logr/logr"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator"
	gwxds "github.com/solo-io/gloo/projects/gateway2/xds"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"
	gloot "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type xdsserver struct {
	c             cache.SnapshotCache
	syncer        *gwxds.XdsSyncer
	cli           client.Client
	scheme        *runtime.Scheme
	inputChannels *gwxds.XdsInputChannels
	log           logr.Logger
}

func NewServer(ctx context.Context, devMode bool, port uint16, inputChannels *gwxds.XdsInputChannels, cli client.Client, scheme *runtime.Scheme) (*xdsserver, error) {
	grpcServer := grpc.NewServer()

	addr := fmt.Sprintf(":%d", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()

	go grpcServer.Serve(lis)
	snapshotCache := newAdsSnapshotCache(ctx)
	xdsServer := server.NewServer(ctx, snapshotCache, nil)
	reflection.Register(grpcServer)

	xds.SetupEnvoyXds(grpcServer, xdsServer, snapshotCache)

	glooTranslator := newGlooTranslator(ctx)

	var sanz sanitizer.XdsSanitizers

	syncer := gwxds.NewXdsSyncer(glooTranslator, sanz, snapshotCache, false, inputChannels)
	s := &xdsserver{
		scheme:        scheme,
		cli:           cli,
		c:             snapshotCache,
		syncer:        syncer,
		inputChannels: inputChannels,
		log:           log.FromContext(ctx).WithValues("xds","gw2"),
	}

	go syncer.SyncXdsOnEvent(ctx, s.onXdsResult)

	if devMode {
		go syncer.ServeXdsSnapshots()
	}

	return s, nil
}

type nodeNameNsHasher struct{}

func (h *nodeNameNsHasher) ID(node *envoy_config_core_v3.Node) string {
	if node.GetMetadata() != nil {
		gatewayValue := node.GetMetadata().GetFields()["gateway"].GetStructValue()
		if gatewayValue != nil {
			name := gatewayValue.GetFields()["name"]
			ns := gatewayValue.GetFields()["namespace"]
			if name != nil && ns != nil {
				return fmt.Sprintf("%v~%v", ns.GetStringValue(), name.GetStringValue())
			}
		}
	}

	return xds.FallbackNodeCacheKey
}

func newAdsSnapshotCache(ctx context.Context) cache.SnapshotCache {
	settings := cache.CacheSettings{
		Ads:    true,
		Hash:   &nodeNameNsHasher{},
		Logger: contextutils.LoggerFrom(ctx),
	}
	return cache.NewSnapshotCache(settings)
}

func newGlooTranslator(ctx context.Context) gloot.Translator {

	settings := &gloov1.Settings{}
	opts := bootstrap.Opts{}
	return gloot.NewDefaultTranslator(settings, registry.GetPluginRegistryFactory(opts)(ctx))

}

func (s *xdsserver) onXdsResult(res gwxds.XdsSyncResult) {
	s.log.Info("got result", "res", res)
}

func (s *xdsserver) Kick(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("kicking translation")
	var gwl apiv1.GatewayList
	err := s.cli.List(ctx, &gwl)
	if err != nil {
		return err
	}
	queries := query.NewData(s.cli, s.scheme)
	t := translator.NewTranslator()
	var proxies gwxds.ProxyInputs
	for _, gw := range gwl.Items {
		rm := &reports.ReportMap{Gateways: make(map[string]*reports.GatewayReport)}
		r := reports.NewReporter(rm)

		proxy := t.TranslateProxy(ctx, &gw, queries, r)
		if proxy != nil {
			proxies.Proxies = append(proxies.Proxies, proxy)
			//TODO: handle reports and process statuses
		}
	}
	log.Info("updating proxy inputs", "numproxies", len(proxies.Proxies))
	s.inputChannels.UpdateProxyInputs(ctx, proxies)

	return nil
}
