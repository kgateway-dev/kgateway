package syncer

import (
	"context"

	"k8s.io/apimachinery/pkg/util/sets"

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
	whitelistNamespaces := sets.NewString()
	blacklistNamespaces := sets.NewString()
	for _, namespace := range namespaces {
		if isBlacklistedNamespace(namespace) {
			blacklistNamespaces.Insert(namespace.Name)
		}
		if isWhitelisted(namespace.Labels) {
			whitelistNamespaces.Insert(namespace.Name)
		}
	}

	switch fdsMode {
	case v1.Settings_DiscoveryOptions_BLACKLIST:
		return filterUpstreamsBlacklist(upstreams, blacklistNamespaces)
	case v1.Settings_DiscoveryOptions_WHITELIST:
		return filterUpstreamsWhitelist(upstreams, whitelistNamespaces, blacklistNamespaces)
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

func filterUpstreamsBlacklist(upstreams v1.UpstreamList, blacklistedNamespaces sets.String) v1.UpstreamList {
	var filtered v1.UpstreamList
	for _, us := range upstreams {
		if shouldIncludeUpstreamInBlacklistMode(us, blacklistedNamespaces) {
			filtered = append(filtered, us)
		}
	}
	return filtered
}

func shouldIncludeUpstreamInBlacklistMode(us *v1.Upstream, blacklistedNamespaces sets.String) bool {
	inBlacklistedNamespace := blacklistedNamespaces.Has(getUpstreamNamespace(us))
	blacklisted := isBlacklisted(us.Metadata.Labels)
	whitelisted := isWhitelisted(us.Metadata.Labels)

	return (!inBlacklistedNamespace || whitelisted) && !blacklisted
}

func filterUpstreamsWhitelist(upstreams v1.UpstreamList, whitelistedNamespaces, blacklistedNamespaces sets.String) (filtered v1.UpstreamList) {
	for _, us := range upstreams {
		inWhitelistedNamespace := whitelistedNamespaces.Has(getUpstreamNamespace(us))
		blacklisted := isBlacklisted(us.Metadata.Labels)
		whitelisted := isWhitelisted(us.Metadata.Labels)

		// if an upstream is AWS, then include it only if it would be included in blacklist mode (https://github.com/solo-io/solo-projects/issues/1339)
		// otherwise, include the upstream only if it is *not* AWS, and either condition holds:
		//   - the upstream is in a whitelisted namespace and not explicitly blacklisted
		//   - the upstream itself is explicitly whitelisted
		shouldIncludeAwsUpstream := us.GetAws() != nil && shouldIncludeUpstreamInBlacklistMode(us, blacklistedNamespaces)
		shouldIncludeNonAwsUpstream := us.GetAws() == nil && ((inWhitelistedNamespace && !blacklisted) || whitelisted)

		if shouldIncludeAwsUpstream || shouldIncludeNonAwsUpstream {
			filtered = append(filtered, us)
		}
	}
	return filtered
}

// TODO: The way we resolve namespace is a bit confusing -- using the service namespace if the upstream is a kube service, or the upstream namespace otherwise
func getUpstreamNamespace(us *v1.Upstream) string {
	if kubeSpec := us.GetKube(); kubeSpec != nil {
		return kubeSpec.ServiceNamespace
	}
	return us.Metadata.Namespace
}
