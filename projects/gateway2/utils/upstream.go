package utils

import (
	"fmt"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

func GetHostnameForUpstream(us *v1.Upstream) string {
	// get the upstream name and namespace
	// TODO: suppport other suffixes that are not cluster.local

	switch uptype := us.GetUpstreamType().(type) {
	case *v1.Upstream_Kube:
		return fmt.Sprintf("%s.%s.svc.cluster.local", uptype.Kube.GetServiceName(), uptype.Kube.GetServiceNamespace())
	}
	return ""
}
