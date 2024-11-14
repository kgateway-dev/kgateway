package crds

import (
	"context"
	"fmt"

	istiokube "istio.io/istio/pkg/kube"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/solo-io/gloo/projects/gateway2/wellknown"
)

const (
	// GatewayClass is the name of the GatewayClass CRD.
	GatewayClass = "gatewayclasses.gateway.networking.k8s.io"
	// Gateway is the name of the Gateway CRD.
	Gateway = "gateways.gateway.networking.k8s.io"
	// HTTPRoute is the name of the HTTPRoute CRD.
	HTTPRoute = "httproutes.gateway.networking.k8s.io"
	// TCPRoute is the name of the TCPRoute CRD.
	TCPRoute = "tcproutes.gateway.networking.k8s.io"
	// ReferenceGrant is the name of the ReferenceGrant CRD.
	ReferenceGrant = "referencegrants.gateway.networking.k8s.io"
	// crdResource is the name of the CustomResourceDefinition resource.
	crdResource = "customresourcedefinitions"
)

var (
	// StandardChannel defines the set of supported Gateway API CRDs from the standard release channel
	// with the discovered annotations for each CRD.
	StandardChannel = CrdToAnnotation{
		GatewayClass:   make(map[string]string),
		Gateway:        make(map[string]string),
		HTTPRoute:      make(map[string]string),
		ReferenceGrant: make(map[string]string),
	}

	// ExperimentalChannel defines the set of supported Gateway API CRDs from the experimental release channel
	// with the discovered annotations for each CRD.
	ExperimentalChannel = CrdToAnnotation{
		TCPRoute: make(map[string]string),
	}

	GVR = extv1.SchemeGroupVersion.WithResource(crdResource)
)

// CrdToAnnotation is a map of CRD annotations keyed by CRD name.
type CrdToAnnotation map[string]map[string]string

// GetGatewayCRDs returns a map of Gateway API CRDs with the discovered annotations for each.
// It queries the Kubernetes API for each supported CRD in the standard and experimental channels,
// verifies its presence, and retrieves its annotations, if any exist.
func GetGatewayCRDs(ctx context.Context, client istiokube.Client) (*CrdToAnnotation, error) {
	crds := make(CrdToAnnotation)

	// Helper function to load CRD annotations by channel
	loadCRDAnnotations := func(channel CrdToAnnotation) error {
		for crdName := range channel {
			resource, err := client.Dynamic().Resource(GVR).Get(ctx, crdName, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("failed to load CRD %s: %w", crdName, err)
			}

			crd := new(extv1.CustomResourceDefinition)
			if err = runtime.DefaultUnstructuredConverter.FromUnstructured(resource.UnstructuredContent(), crd); err != nil {
				return fmt.Errorf("failed to convert CRD %s: %w", crdName, err)
			}

			// Retrieve and store annotations if they exist
			annotations := crd.Annotations
			if annotations == nil {
				annotations = make(map[string]string)
			}
			crds[crdName] = annotations
		}
		return nil
	}

	// Load annotations for standard and experimental channels
	if err := loadCRDAnnotations(StandardChannel); err != nil {
		return nil, err
	}
	if err := loadCRDAnnotations(ExperimentalChannel); err != nil {
		return nil, err
	}

	if len(crds) == 0 {
		return nil, fmt.Errorf("no Gateway API CRDs found in the cluster")
	}

	return &crds, nil
}

// IsSupportedVersion checks if the provided CRD version is supported.
func IsSupportedVersion(version string) bool {
	supportedVersions := sets.NewString(wellknown.SupportedVersions...)
	return supportedVersions.Has(version)
}

// IsSupported checks if the CRD is supported based on the provided name.
func IsSupported(name string) bool {
	return name == GatewayClass ||
		name == Gateway ||
		name == HTTPRoute ||
		name == ReferenceGrant ||
		name == TCPRoute
}
