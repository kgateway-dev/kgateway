package collections

import (
	"context"
	"log/slog"

	"istio.io/istio/pkg/config/schema/gvr"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

var promotedTCPRouteGVR = wellknown.TCPRouteV1GVR

type servedTCPRouteVersions struct {
	Promoted      bool
	PreV1         bool
	Authoritative bool
}

func fallbackTCPRouteVersions() servedTCPRouteVersions {
	return servedTCPRouteVersions{
		Promoted: true,
		PreV1:    true,
	}
}

// preV1TCPRouteWatchGVRs returns the pre-v1 TCPRoute API versions that should
// be watched for the current discovery result. When discovery is authoritative
// and the promoted v1 version is served, skip the pre-v1 watch to avoid
// duplicate logical TCPRoutes.
func preV1TCPRouteWatchGVRs(versions servedTCPRouteVersions) []schema.GroupVersionResource {
	if !versions.PreV1 || (versions.Authoritative && versions.Promoted) {
		return nil
	}
	return []schema.GroupVersionResource{gvr.TCPRoute}
}

// tcpRouteWriteGVRs returns the API versions status writes may go through, most preferred
// first. Every served version shares one storage object, so a write through any of them
// updates the same status.
//
// When discovery was authoritative the answer is exactly one version. When it was not — the
// API server was unavailable, or the CRD is not installed yet, which is precisely the case
// the watch path deliberately self-heals through delayed informers — we cannot know which
// versions the CRD will serve once it appears. Committing to the promoted v1 guess would
// send every write through a client for a never-served version whose Get returns nil, so
// Writer.ApplyStatus would skip silently and no TCPRoute would carry status until the pod
// restarted. Instead we return every version we also watch and let the writer dispatch to
// the one whose informer actually holds the object.
//
// watchPreV1 must match whether the caller watches the pre-v1 versions: a version nothing
// watches can never hold the object, so listing it would only add a dead client.
func tcpRouteWriteGVRs(versions servedTCPRouteVersions, watchPreV1 bool) []schema.GroupVersionResource {
	if versions.Authoritative {
		if !versions.Promoted && versions.PreV1 {
			return []schema.GroupVersionResource{wellknown.TCPRouteGVR}
		}
		// Either the promoted version is served, or the CRD serves no version we
		// understand and no write can succeed through any of them. Name the promoted
		// version in both cases, matching tlsRouteWriteGVRs' degenerate answer.
		return []schema.GroupVersionResource{promotedTCPRouteGVR}
	}

	gvrs := []schema.GroupVersionResource{promotedTCPRouteGVR}
	if watchPreV1 {
		gvrs = append(gvrs, preV1TCPRouteWatchGVRs(versions)...)
	}
	return gvrs
}

// getServedTCPRouteVersions resolves which TCPRoute API versions are currently
// served by the cluster. When discovery is unavailable, or the CRD is not yet
// installed, we conservatively allow both promoted and pre-v1 watches so
// startup does not incorrectly disable TCPRoute support before delayed
// informers can recover.
func getServedTCPRouteVersions(ctx context.Context, extClient apiextensionsclient.Interface) servedTCPRouteVersions {
	if extClient == nil {
		// If discovery is unavailable, keep both paths enabled and let the delayed
		// informer logic determine what is actually readable at runtime.
		slog.Warn("no CRD discovery client for TCPRoute; watching and writing status through every known API version until the CRD is observed")
		return fallbackTCPRouteVersions()
	}

	ctx, cancel := context.WithTimeout(ctx, crdLookupTimeout)
	defer cancel()

	crd, err := extClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, wellknown.TCPRouteCRDName, metav1.GetOptions{})
	if err != nil {
		// The CRD may simply not be installed yet. The watch path recovers through delayed
		// informers, and the write path spans every version we watch, but neither can
		// narrow to the version the CRD ends up serving without a restart.
		slog.Warn("could not resolve served TCPRoute API versions; watching and writing status through every known API version until restart",
			"crd", wellknown.TCPRouteCRDName,
			"error", err,
		)
		return fallbackTCPRouteVersions()
	}

	versions := servedTCPRouteVersions{Authoritative: true}
	for _, version := range crd.Spec.Versions {
		if !version.Served {
			continue
		}

		switch version.Name {
		case gwv1.GroupVersion.Version:
			versions.Promoted = true
		case gwv1a2.GroupVersion.Version:
			versions.PreV1 = true
		}
	}

	return versions
}

func convertTCPRouteV1ToV1Alpha2(in *gwv1.TCPRoute) *gwv1a2.TCPRoute {
	if in == nil {
		return nil
	}

	return &gwv1a2.TCPRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gwv1a2.GroupVersion.String(),
			Kind:       wellknown.TCPRouteKind,
		},
		ObjectMeta: *in.ObjectMeta.DeepCopy(),
		Spec: gwv1a2.TCPRouteSpec{
			CommonRouteSpec: in.Spec.CommonRouteSpec,
			Rules:           convertTCPRouteRulesV1ToV1Alpha2(in.Spec.Rules),
		},
		// Status must be carried over: the declarative status writer compares the live
		// status on this converted object against the desired status to decide whether
		// a write is needed.
		Status: gwv1a2.TCPRouteStatus{
			RouteStatus: in.Status.RouteStatus,
		},
	}
}

func convertTCPRouteRulesV1ToV1Alpha2(in []gwv1.TCPRouteRule) []gwv1a2.TCPRouteRule {
	if len(in) == 0 {
		return nil
	}

	out := make([]gwv1a2.TCPRouteRule, 0, len(in))
	for _, rule := range in {
		out = append(out, gwv1a2.TCPRouteRule(rule))
	}
	return out
}
