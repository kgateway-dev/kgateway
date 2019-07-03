package syncer

import (
	"context"

	"github.com/solo-io/gloo/projects/discovery/pkg/fds"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
)

type syncer struct {
	fd *fds.FunctionDiscovery
}

func NewDiscoverySyncer(fd *fds.FunctionDiscovery) v1.DiscoverySyncer {
	s := &syncer{
		fd: fd,
	}
	return s
}

func (s *syncer) Sync(ctx context.Context, snap *v1.DiscoverySnapshot) error {
	ctx = contextutils.WithLogger(ctx, "syncer")
	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("begin sync %v (%v upstreams)", snap.Hash(), len(snap.Upstreams))
	defer logger.Infof("end sync %v", snap.Hash())

	logger.Debugf("%v", snap)

	upstreamsToDetect := filterUpstreamsForDiscovery(snap.Upstreams, snap.Kubenamespaces)

	return s.fd.Update(upstreamsToDetect, snap.Secrets)
}

const (
	FdsLabelKey       = "discovery.solo.io/function_discovery"
	enbledLabelValue  = "enabled"
	disbledLabelValue = "disabled"
)

func filterUpstreamsForDiscovery(upstreams v1.UpstreamList, namespaces kubernetes.KubeNamespaceList) v1.UpstreamList {
	fdsNamespaces := make(map[string]bool)
	for _, ns := range namespaces {
		fdsNamespaces[ns.Name] = shouldDiscoverOnNamespace(ns)
	}
	var filtered v1.UpstreamList
	for _, us := range upstreams {
		if shouldDiscoverOnUpstream(us, fdsNamespaces) {
			filtered = append(filtered, us)
		}
	}
	return filtered
}

// do not run fds on these namespaces unless explicitly enabled
var defaultOffNamespaces = []string{"kube-system", "kube-public"}

func shouldDiscoverOnNamespace(ns *kubernetes.KubeNamespace) bool {
	for _, defaultOff := range defaultOffNamespaces {
		if ns.Name == defaultOff {
			return ns.Labels != nil && ns.Labels[FdsLabelKey] == enbledLabelValue
		}
	}
	return ns.Labels == nil || ns.Labels[FdsLabelKey] != disbledLabelValue
}

func shouldDiscoverOnUpstream(us *v1.Upstream, fdsNamespaces map[string]bool) bool {
	ns := getUpstreamNamespace(us)
	if ns != "" {
		// only apply this filter if namespace is set, otherwise
		// it's not a kube upstream
		if !fdsNamespaces[ns] {
			return false
		}
	}
	return us.Metadata.Labels == nil || us.Metadata.Labels[FdsLabelKey] != disbledLabelValue
}

func getUpstreamNamespace(us *v1.Upstream) string {
	if kubeSpec := us.GetUpstreamSpec().GetKube(); kubeSpec != nil {
		return kubeSpec.ServiceNamespace
	}
	return "" // only applies to kube namespaces currently
}
