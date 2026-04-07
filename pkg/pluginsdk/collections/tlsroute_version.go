package collections

import (
	"log/slog"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

var promotedTLSRouteGVR = schema.GroupVersionResource{
	Group:    wellknown.GatewayGroup,
	Version:  gwv1.GroupVersion.Version,
	Resource: "tlsroutes",
}

var legacyTLSRouteGVR = schema.GroupVersionResource{
	Group:    wellknown.GatewayGroup,
	Version:  wellknown.LegacyTLSRouteVersion,
	Resource: "tlsroutes",
}

type servedTLSRouteVersions struct {
	Promoted      bool
	Legacy        bool
	LegacyGVR     schema.GroupVersionResource
	Authoritative bool
}

func getServedTLSRouteVersions(extClient apiextensionsclient.Interface) servedTLSRouteVersions {
	discovered := getServedVersions(extClient, "tlsroutes.gateway.networking.k8s.io", gwv1.GroupVersion.Version, wellknown.LegacyTLSRouteVersion, gwv1a2.GroupVersion.Version)
	if !discovered.Authoritative {
		return servedTLSRouteVersions{Promoted: true, Legacy: true, LegacyGVR: legacyTLSRouteGVR}
	}

	versions := servedTLSRouteVersions{
		Promoted:      discovered.Served[gwv1.GroupVersion.Version],
		Authoritative: true,
	}
	for _, legacyVersion := range []string{wellknown.LegacyTLSRouteVersion, gwv1a2.GroupVersion.Version} {
		if discovered.Served[legacyVersion] {
			versions.Legacy = true
			versions.LegacyGVR = schema.GroupVersionResource{
				Group:    wellknown.GatewayGroup,
				Version:  legacyVersion,
				Resource: "tlsroutes",
			}
			break
		}
	}

	return versions
}

func convertTLSRouteV1ToV1Alpha2(in *gwv1.TLSRoute) *gwv1a2.TLSRoute {
	if in == nil {
		return nil
	}

	return &gwv1a2.TLSRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gwv1a2.GroupVersion.String(),
			Kind:       wellknown.TLSRouteKind,
		},
		ObjectMeta: *in.ObjectMeta.DeepCopy(),
		Spec: gwv1a2.TLSRouteSpec{
			CommonRouteSpec: gwv1a2.CommonRouteSpec{
				ParentRefs:         in.Spec.ParentRefs,
				UseDefaultGateways: in.Spec.UseDefaultGateways,
			},
			Hostnames: convertTLSRouteHostnamesV1ToV1Alpha2(in.Spec.Hostnames),
			Rules:     convertTLSRouteRulesV1ToV1Alpha2(in.Spec.Rules),
		},
	}
}

func convertLegacyTLSRouteToV1Alpha2(in *unstructured.Unstructured) *gwv1a2.TLSRoute {
	if in == nil {
		return nil
	}

	out := &gwv1a2.TLSRoute{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(in.Object, out); err != nil {
		slog.Warn("ignoring legacy TLSRoute with invalid payload",
			"name", in.GetName(),
			"namespace", in.GetNamespace(),
			"error", err,
		)
		return nil
	}
	out.SetGroupVersionKind(wellknown.TLSRouteGVK)
	return out
}

// ConvertLegacyTLSRouteToV1Alpha2ForStatus normalizes legacy TLSRoute objects
// served from experimental Gateway API bundles so status code can reuse the
// existing gwv1a2-based report builder.
func ConvertLegacyTLSRouteToV1Alpha2ForStatus(in *unstructured.Unstructured) *gwv1a2.TLSRoute {
	return convertLegacyTLSRouteToV1Alpha2(in)
}

func convertTLSRouteHostnamesV1ToV1Alpha2(in []gwv1.Hostname) []gwv1a2.Hostname {
	if len(in) == 0 {
		return nil
	}

	out := make([]gwv1a2.Hostname, 0, len(in))
	for _, hostname := range in {
		out = append(out, gwv1a2.Hostname(hostname))
	}
	return out
}

func convertTLSRouteRulesV1ToV1Alpha2(in []gwv1.TLSRouteRule) []gwv1a2.TLSRouteRule {
	if len(in) == 0 {
		return nil
	}

	out := make([]gwv1a2.TLSRouteRule, 0, len(in))
	for _, rule := range in {
		out = append(out, gwv1a2.TLSRouteRule(rule))
	}
	return out
}
