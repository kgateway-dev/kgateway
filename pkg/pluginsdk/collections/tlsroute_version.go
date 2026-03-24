package collections

import (
	"context"
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
	Version:  gwv1a2.GroupVersion.Version,
	Resource: "tlsroutes",
}

type servedTLSRouteVersions struct {
	Promoted      bool
	Legacy        bool
	Authoritative bool
}

func getServedTLSRouteVersions(extClient apiextensionsclient.Interface) servedTLSRouteVersions {
	if extClient == nil {
		return servedTLSRouteVersions{Promoted: true, Legacy: true}
	}

	crd, err := extClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.Background(), "tlsroutes.gateway.networking.k8s.io", metav1.GetOptions{})
	if err != nil {
		return servedTLSRouteVersions{Promoted: true, Legacy: true}
	}

	versions := servedTLSRouteVersions{Authoritative: true}
	for _, version := range crd.Spec.Versions {
		if !version.Served {
			continue
		}
		switch version.Name {
		case gwv1.GroupVersion.Version:
			versions.Promoted = true
		case gwv1a2.GroupVersion.Version:
			versions.Legacy = true
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
	if out.GroupVersionKind().Empty() {
		out.SetGroupVersionKind(legacyTLSRouteGVR.GroupVersion().WithKind(wellknown.TLSRouteKind))
	}
	return out
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
