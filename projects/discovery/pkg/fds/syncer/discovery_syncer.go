package syncer

import (
	"context"

	"github.com/solo-io/gloo/projects/discovery/pkg/fds"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
)

type syncer struct {
	fd      *fds.FunctionDiscovery
	fdsMode v1.Settings_DiscoveryOptions_FdsMode
}

func NewDiscoverySyncer(fd *fds.FunctionDiscovery, fdsMode v1.Settings_DiscoveryOptions_FdsMode) v1.DiscoverySyncer {
	s := &syncer{
		fd:      fd,
		fdsMode: fdsMode,
	}
	return s
}

func (s *syncer) Sync(ctx context.Context, snap *v1.DiscoverySnapshot) error {
	ctx = contextutils.WithLogger(ctx, "syncer")
	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("begin sync %v (%v upstreams)", snap.Hash(), len(snap.Upstreams))
	defer logger.Infof("end sync %v", snap.Hash())

	logger.Debugf("%v", snap)

	upstreamsToDetect := filterUpstreamsForDiscovery(s.fdsMode, snap.Upstreams, snap.Kubenamespaces)

	return s.fd.Update(upstreamsToDetect, snap.Secrets)
}

const (
	FdsLabelKey       = "discovery.solo.io/function_discovery"
	enbledLabelValue  = "enabled"
	disbledLabelValue = "disabled"
)

func filterUpstreamsForDiscovery(fdsMode v1.Settings_DiscoveryOptions_FdsMode, upstreams v1.UpstreamList, namespaces kubernetes.KubeNamespaceList) v1.UpstreamList {
	fdsNamespaces := make(map[string]bool)
	for _, ns := range namespaces {
		fdsNamespaces[ns.Name] = shouldDiscoverOnNamespace(fdsMode, ns)
	}
	var filtered v1.UpstreamList
	for _, us := range upstreams {
		if shouldDiscoverOnUpstream(fdsMode, us, fdsNamespaces) {
			filtered = append(filtered, us)
		}
	}
	return filtered
}

// do not run fds on these namespaces unless explicitly enabled
var blacklistedNamespaces = []string{"kube-system", "kube-public"}

func shouldDiscoverOnNamespace(fdsMode v1.Settings_DiscoveryOptions_FdsMode, ns *kubernetes.KubeNamespace) bool {
	switch fdsMode {
	case v1.Settings_DiscoveryOptions_WHITELIST:
	case v1.Settings_DiscoveryOptions_BLACKLIST:
		for _, defaultOff := range blacklistedNamespaces {
			if ns.Name == defaultOff {
				return shouldDiscoverLabels(v1.Settings_DiscoveryOptions_WHITELIST, ns.Labels)
			}
		}
	default:
		panic("invalid fds mode: " + fdsMode.String())
	}
	return shouldDiscoverLabels(fdsMode, ns.Labels)
}

func shouldDiscoverOnUpstream(fdsMode v1.Settings_DiscoveryOptions_FdsMode, us *v1.Upstream, fdsNamespaces map[string]bool) bool {
	ns := getUpstreamNamespace(us)
	if ns == "" {
		// don't filter non-kube upstreams
		return true
	}
	// if namespace is enabled, look for the disabled label
	if fdsNamespaces[ns] {
		return shouldDiscoverLabels(v1.Settings_DiscoveryOptions_BLACKLIST, us.GetMetadata().Labels)
	}
	// namesapce is disabled, look for the enabled label
	return shouldDiscoverLabels(v1.Settings_DiscoveryOptions_WHITELIST, us.GetMetadata().Labels)
}

func getUpstreamNamespace(us *v1.Upstream) string {
	if kubeSpec := us.GetUpstreamSpec().GetKube(); kubeSpec != nil {
		return kubeSpec.ServiceNamespace
	}
	return "" // only applies to kube namespaces currently
}

func shouldDiscoverLabels(fdsMode v1.Settings_DiscoveryOptions_FdsMode, labels map[string]string) bool {
	switch fdsMode {
	case v1.Settings_DiscoveryOptions_WHITELIST:
		return labels != nil && labels[FdsLabelKey] == enbledLabelValue
	case v1.Settings_DiscoveryOptions_BLACKLIST:
		return labels == nil || labels[FdsLabelKey] != disbledLabelValue
	default:
		panic("invalid fds mode: " + fdsMode.String())
	}
}
