package utils

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"knative.dev/pkg/network"
)

func GetHostnameForUpstream(us *v1.Upstream) string {
	// get the upstream name and namespace
	switch uptype := us.GetUpstreamType().(type) {
	case *v1.Upstream_Kube:
		return network.GetServiceHostname(uptype.Kube.GetServiceName(), uptype.Kube.GetServiceNamespace())
	case *v1.Upstream_Static:
		hosts := uptype.Static.GetHosts()
		if len(hosts) == 0 {
			return ""
		}
		return hosts[0].GetAddr()
	}
	return ""
}

func GetPortForUpstream(us *v1.Upstream) uint32 {
	// get the upstream name and namespace
	// TODO: suppport other suffixes that are not cluster.local

	switch uptype := us.GetUpstreamType().(type) {
	case *v1.Upstream_Kube:
		return uptype.Kube.GetServicePort()
	case *v1.Upstream_Static:
		hosts := uptype.Static.GetHosts()
		if len(hosts) == 0 {
			return 0
		}
		return hosts[0].GetPort()
	}
	return 0
}
