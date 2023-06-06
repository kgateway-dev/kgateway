package services

import (
	"github.com/solo-io/gloo/test/services/envoy"
)

type EnvoyInstance = envoy.Instance
type EnvoyFactory = envoy.Factory
type EnvoyBootstrapBuilder = envoy.BootstrapBuilder

const DefaultProxyName = envoy.DefaultProxyName
