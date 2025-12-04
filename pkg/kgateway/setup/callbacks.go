package setup

import (
	"context"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
)

func chainCallbacks(cbs ...xdsserver.Callbacks) xdsserver.Callbacks {
	return &callbacksChain{Callbacks: cbs}
}

type callbacksChain struct {
	Callbacks []xdsserver.Callbacks
}

func (c *callbacksChain) OnDeltaStreamClosed(s int64, n *envoycorev3.Node) {
	for _, cb := range c.Callbacks {
		cb.OnDeltaStreamClosed(s, n)
	}
}

func (c *callbacksChain) OnDeltaStreamOpen(ctx context.Context, streamID int64, typeURL string) error {
	for _, cb := range c.Callbacks {
		if err := cb.OnDeltaStreamOpen(ctx, streamID, typeURL); err != nil {
			return err
		}
	}
	return nil
}

func (c *callbacksChain) OnFetchRequest(ctx context.Context, req *discoveryv3.DiscoveryRequest) error {
	for _, cb := range c.Callbacks {
		if err := cb.OnFetchRequest(ctx, req); err != nil {
			return err
		}
	}
	return nil
}

func (c *callbacksChain) OnFetchResponse(req *discoveryv3.DiscoveryRequest, resp *discoveryv3.DiscoveryResponse) {
	for _, cb := range c.Callbacks {
		cb.OnFetchResponse(req, resp)
	}
}

func (c *callbacksChain) OnStreamClosed(streamID int64, node *envoycorev3.Node) {
	for _, cb := range c.Callbacks {
		cb.OnStreamClosed(streamID, node)
	}
}

func (c *callbacksChain) OnStreamDeltaRequest(streamID int64, deltaReq *discoveryv3.DeltaDiscoveryRequest) error {
	for _, cb := range c.Callbacks {
		if err := cb.OnStreamDeltaRequest(streamID, deltaReq); err != nil {
			return err
		}
	}
	return nil
}

func (c *callbacksChain) OnStreamDeltaResponse(streamID int64, deltaReq *discoveryv3.DeltaDiscoveryRequest, deltaResp *discoveryv3.DeltaDiscoveryResponse) {
	for _, cb := range c.Callbacks {
		cb.OnStreamDeltaResponse(streamID, deltaReq, deltaResp)
	}
}

func (c *callbacksChain) OnStreamOpen(ctx context.Context, streamID int64, typeURL string) error {
	for _, cb := range c.Callbacks {
		if err := cb.OnStreamOpen(ctx, streamID, typeURL); err != nil {
			return err
		}
	}
	return nil
}

func (c *callbacksChain) OnStreamRequest(streamID int64, req *discoveryv3.DiscoveryRequest) error {
	for _, cb := range c.Callbacks {
		if err := cb.OnStreamRequest(streamID, req); err != nil {
			return err
		}
	}
	return nil
}

func (c *callbacksChain) OnStreamResponse(ctx context.Context, streamID int64, req *discoveryv3.DiscoveryRequest, resp *discoveryv3.DiscoveryResponse) {
	for _, cb := range c.Callbacks {
		cb.OnStreamResponse(ctx, streamID, req, resp)
	}
}
