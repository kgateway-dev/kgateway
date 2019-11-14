package syncer

import (
	"context"

	"github.com/solo-io/gloo/pkg/utils/syncutil"
	"go.uber.org/zap/zapcore"

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

	// stringifying the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	if contextutils.GetLogLevel() == zapcore.DebugLevel {
		logger.Debug(syncutil.StringifySnapshot(snap))
	}

	upstreamsToDetect := filterUpstreamsForDiscovery(s.fdsMode, snap.Upstreams, snap.Kubenamespaces)

	return s.fd.Update(upstreamsToDetect, snap.Secrets)
}

const (
	FdsLabelKey       = "discovery.solo.io/function_discovery"
	enbledLabelValue  = "enabled"
	disbledLabelValue = "disabled"
)

func filterUpstreamsForDiscovery(fdsMode v1.Settings_DiscoveryOptions_FdsMode, upstreams v1.UpstreamList, namespaces kubernetes.KubeNamespaceList) v1.UpstreamList {
	switch fdsMode {
	case v1.Settings_DiscoveryOptions_BLACKLIST:
		return filterUpstreamsBlacklist(upstreams, namespaces)
	case v1.Settings_DiscoveryOptions_WHITELIST:
		return filterUpstreamsWhitelist(upstreams, namespaces)
	}
	panic("invalid fds mode: " + fdsMode.String())
}

func isBlacklisted(labels map[string]string) bool {
	return labels != nil && labels[FdsLabelKey] == disbledLabelValue
}

func isWhitelisted(labels map[string]string) bool {
	return labels != nil && labels[FdsLabelKey] == enbledLabelValue
}

// do not run fds on these namespaces unless explicitly enabled
var defaultBlacklistedNamespaces = []string{"kube-system", "kube-public"}

func isBlacklistedNamespace(ns *kubernetes.KubeNamespace) bool {
	if isBlacklisted(ns.Labels) {
		return true
	}
	for _, defaultBlacklistedNs := range defaultBlacklistedNamespaces {
		if ns.Name == defaultBlacklistedNs {
			return !isWhitelisted(ns.Labels)
		}
	}
	return false
}

func filterUpstreamsBlacklist(upstreams v1.UpstreamList, namespaces kubernetes.KubeNamespaceList) v1.UpstreamList {
	blacklistedNamespaces := make(map[string]bool)

	for _, ns := range namespaces {
		if isBlacklistedNamespace(ns) {
			blacklistedNamespaces[ns.Name] = true
		}
	}

	var filtered v1.UpstreamList
	for _, us := range upstreams {
		inBlacklistedNamespace := blacklistedNamespaces[getUpstreamNamespace(us)]
		blacklisted := isBlacklisted(us.Metadata.Labels)
		whitelisted := isWhitelisted(us.Metadata.Labels)
		if (inBlacklistedNamespace && !whitelisted) || blacklisted {
			continue
		}
		filtered = append(filtered, us)
	}
	return filtered
}

func filterUpstreamsWhitelist(upstreams v1.UpstreamList, namespaces kubernetes.KubeNamespaceList) v1.UpstreamList {
	whitelistedNamespaces := make(map[string]bool)

	for _, ns := range namespaces {
		if isWhitelisted(ns.Labels) {
			whitelistedNamespaces[ns.Name] = true
		}
	}

	var filtered v1.UpstreamList
	for _, us := range upstreams {
		inWhitelistedNamespace := whitelistedNamespaces[getUpstreamNamespace(us)]
		blacklisted := isBlacklisted(us.Metadata.Labels)
		whitelisted := isWhitelisted(us.Metadata.Labels)
		if (inWhitelistedNamespace && !blacklisted) || whitelisted {
			filtered = append(filtered, us)
		}
	}
	return filtered
}

func getUpstreamNamespace(us *v1.Upstream) string {
	if kubeSpec := us.GetKube(); kubeSpec != nil {
		return kubeSpec.ServiceNamespace
	}
	return "" // only applies to kube namespaces currently
}
