package krtxds_test

import (
	"context"
	"net"
	"testing"

	"github.com/agentgateway/agentgateway/go/api"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	istioxds "istio.io/istio/pilot/pkg/xds"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/ptr"
	"istio.io/istio/pkg/test"
	"istio.io/istio/pkg/test/util/assert"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/agentgatewaysyncer"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/agentgatewaysyncer/krtxds"
	agwir "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

type Fake struct {
	Server      *krtxds.DiscoveryServer
	Addresses   krt.StaticCollection[agentgatewaysyncer.Address]
	Resources   krt.StaticCollection[agwir.AgwResource]
	BufListener *bufconn.Listener
	t           *testing.T
}

func NewFakeDiscoveryServer(t *testing.T, initialAddress ...agentgatewaysyncer.Address) Fake {
	stop := test.NewStop(t)
	opts := krtutil.NewKrtOptions(stop, new(krt.DebugHandler))
	xdsAddress := krt.NewStaticCollection[agentgatewaysyncer.Address](nil, initialAddress, opts.ToOptions("address")...)
	xdsResource := krt.NewStaticCollection[agwir.AgwResource](nil, nil, opts.ToOptions("resource")...)
	agwResourcesByGateway := func(resource agwir.AgwResource) types.NamespacedName {
		return resource.Gateway
	}
	reg := []krtxds.Registration{
		krtxds.Collection[agentgatewaysyncer.Address, *api.Address](xdsAddress, opts),
		krtxds.PerGatewayCollection[agwir.AgwResource, *api.Resource](xdsResource, agwResourcesByGateway, opts),
	}
	// we won't need a mock nack event publisher for this testing, so we pass nil
	s := krtxds.NewDiscoveryServer(opts.Debugger, nil, reg...)
	s.Start(stop)
	xdsAddress.WaitUntilSynced(stop)
	xdsResource.WaitUntilSynced(stop)
	kube.WaitForCacheSync("test", stop, s.IsServerReady)

	buffer := 1024 * 1024
	listener := bufconn.Listen(buffer)

	grpcServer := grpc.NewServer()
	discovery.RegisterAggregatedDiscoveryServiceServer(grpcServer, s)
	go func() {
		if err := grpcServer.Serve(listener); err != nil && !(err == grpc.ErrServerStopped || err.Error() == "closed") {
			t.Fatal(err)
		}
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})

	return Fake{
		Server:      s,
		t:           t,
		BufListener: listener,
		Addresses:   xdsAddress,
		Resources:   xdsResource,
	}
}

// ConnectDeltaADS starts a Delta ADS connection to the server. It will automatically be cleaned up when the test ends
func (f Fake) ConnectDeltaADS() *istioxds.DeltaAdsTest {
	//nolint:staticcheck // for testing
	conn, err := grpc.Dial("buffcon",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		//nolint:staticcheck // for testing
		grpc.WithBlock(),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return f.BufListener.Dial()
		}))
	if err != nil {
		f.t.Fatalf("failed to connect: %v", err)
	}
	return istioxds.NewDeltaAdsTest(f.t, conn)
}

var (
	testWorkload1 = agentgatewaysyncer.Address{
		Workload: ptr.Of(agentgatewaysyncer.PrecomputeWorkload(agentgatewaysyncer.WorkloadInfo{Workload: &api.Workload{Uid: "wl1"}})),
	}
)

func TestEmptyXDS(t *testing.T) {
	s := NewFakeDiscoveryServer(t)
	ads := s.ConnectDeltaADS().WithType(translator.TargetTypeAddressUrl)
	ads.Request(nil)
	ads.ExpectEmptyResponse()
}

func TestXDS(t *testing.T) {
	s := NewFakeDiscoveryServer(t, testWorkload1)
	ads := s.ConnectDeltaADS().WithType(translator.TargetTypeAddressUrl)
	ads.RequestResponseAck(nil)
}

func TestXDSUpdate(t *testing.T) {
	s := NewFakeDiscoveryServer(t, testWorkload1)
	ads := s.ConnectDeltaADS().WithType(translator.TargetTypeAddressUrl)
	ads.RequestResponseAck(nil)

	wl1Updated := agentgatewaysyncer.Address{
		Workload: ptr.Of(agentgatewaysyncer.PrecomputeWorkload(agentgatewaysyncer.WorkloadInfo{Workload: &api.Workload{Uid: "wl1", ClusterId: "cluster1"}})),
	}
	s.Addresses.UpdateObject(wl1Updated)
	resp := ads.ExpectResponse()
	assert.Equal(t, len(resp.Resources), 1)
	assert.Equal(t, len(resp.RemovedResources), 0)
	ads.ExpectNoResponse()

	s.Addresses.DeleteObject("wl1")
	resp = ads.ExpectResponse()
	assert.Equal(t, len(resp.Resources), 0)
	assert.Equal(t, len(resp.RemovedResources), 1)
}
