package translator

import (
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

func ValidateAuthConfigs(snap *v1.ApiSnapshot, filter *envoylistener.Filter) error {
	//upstreams := snap.AuthConfigs
	return nil
}
