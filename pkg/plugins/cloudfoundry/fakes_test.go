package cloudfoundry_test

import (
	"context"
	"sync"

	copilotapi "code.cloudfoundry.org/copilot/api"
	grpc "google.golang.org/grpc"
)

type FakeIstioClient struct {
	FakeResponse      *copilotapi.RoutesResponse
	FakeResponseError error

	values sync.RWMutex
}

func (f *FakeIstioClient) Close() error {
	return nil
}

func (f *FakeIstioClient) Health(ctx context.Context, in *copilotapi.HealthRequest, opts ...grpc.CallOption) (*copilotapi.HealthResponse, error) {
	panic("this should not be called")
}

func (f *FakeIstioClient) Routes(ctx context.Context, in *copilotapi.RoutesRequest, opts ...grpc.CallOption) (*copilotapi.RoutesResponse, error) {
	f.values.RLock()
	defer f.values.RUnlock()
	return f.FakeResponse, f.FakeResponseError
}

func (f *FakeIstioClient) SetFakeResponse(hostname, address string, port uint32) {
	f.values.Lock()
	defer f.values.Unlock()
	f.FakeResponse = FakeResponse(hostname, address, port)
}

func FakeResponse(hostname, address string, port uint32) *copilotapi.RoutesResponse {
	var resp copilotapi.RoutesResponse
	resp.Backends = make(map[string]*copilotapi.BackendSet)
	resp.Backends[hostname] = new(copilotapi.BackendSet)
	resp.Backends[hostname].Backends = []*copilotapi.Backend{
		{
			Address: address,
			Port:    port,
		},
	}
	return &resp
}
