package setup

import (
	"context"

	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	xdsserver "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
)

type MutltiCallbacks []xdsserver.Callbacks

// OnStreamOpen is called once an xDS stream is open with a stream ID and the type URL (or "" for ADS).
// Returning an error will end processing and close the stream. OnStreamClosed will still be called.
func (mc MutltiCallbacks) OnStreamOpen(ctx context.Context, sid int64, url string) error {
	for _, c := range mc {
		err := c.OnStreamOpen(ctx, sid, url)
		if err != nil {
			return err
		}
	}
	return nil
}

// OnStreamClosed is called immediately prior to closing an xDS stream with a stream ID.
func (mc MutltiCallbacks) OnStreamClosed(sid int64) {
	for _, c := range mc {
		c.OnStreamClosed(sid)
	}
}

// OnStreamRequest is called once a request is received on a stream.
// Returning an error will end processing and close the stream. OnStreamClosed will still be called.
func (mc MutltiCallbacks) OnStreamRequest(sid int64, dr *envoy_service_discovery_v3.DiscoveryRequest) error {
	for _, c := range mc {
		err := c.OnStreamRequest(sid, dr)
		if err != nil {
			return err
		}
	}
	return nil
}

// OnStreamResponse is called immediately prior to sending a response on a stream.
func (mc MutltiCallbacks) OnStreamResponse(sid int64, dr *envoy_service_discovery_v3.DiscoveryRequest, dresp *envoy_service_discovery_v3.DiscoveryResponse) {
	for _, c := range mc {
		c.OnStreamResponse(sid, dr, dresp)
	}
}

// OnFetchRequest is called for each Fetch request. Returning an error will end processing of the
// request and respond with an error.
func (mc MutltiCallbacks) OnFetchRequest(ctx context.Context, dr *envoy_service_discovery_v3.DiscoveryRequest) error {
	for _, c := range mc {
		err := c.OnFetchRequest(ctx, dr)
		if err != nil {
			return err
		}
	}
	return nil
}

// OnFetchResponse is called immediately prior to sending a response.
func (mc MutltiCallbacks) OnFetchResponse(dr *envoy_service_discovery_v3.DiscoveryRequest, dresp *envoy_service_discovery_v3.DiscoveryResponse) {
	for _, c := range mc {
		c.OnFetchResponse(dr, dresp)
	}
}
