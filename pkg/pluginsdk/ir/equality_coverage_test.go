package ir

// equality_coverage_test.go covers the IR types whose Equals implementations
// deliberately omit fields. The tests model representative updates to those
// fields together with the versions or hashes that Equals compares.
//
// See test/testutils/equalstest for the harness API.

import (
	"errors"
	"testing"

	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/test/testutils/equalstest"
)

// errTestRuleResolution stands in for an error recorded during IR construction.
var errTestRuleResolution = errors.New("backend resolution failed")

// MARK: routes

// routeSpecFields lists the fields that are verbatim copies of the source
// object's spec. They are exempt from direct comparison because any change to
// them bumps the source object's generation, which versionEquals observes;
// TestRouteIREqualsObservesSpecChangeViaGeneration proves that.
var routeSpecFields = []string{"ParentRefs", "Hostnames"}

// embeddedObjectSourceFields are the ObjectSource fields the harness flattens;
// each test covers the embedding as a whole via an "ObjectSource" case.
var embeddedObjectSourceFields = []string{"Group", "Kind", "Namespace", "Name"}

func baseHarnessHTTPRouteObj() *gwv1.HTTPRoute {
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-route",
			Namespace:  "default",
			UID:        "route-uid-1",
			Generation: 1,
		},
		Spec: gwv1.HTTPRouteSpec{
			Hostnames: []gwv1.Hostname{"example.com"},
		},
	}
}

func baseHarnessHttpRouteIR() HttpRouteIR {
	return HttpRouteIR{
		ObjectSource: ObjectSource{
			Group:     gwv1.GroupVersion.Group,
			Kind:      "HTTPRoute",
			Namespace: "default",
			Name:      "my-route",
		},
		SourceObject: baseHarnessHTTPRouteObj(),
		ParentRefs:   []gwv1.ParentReference{{Name: "my-gateway"}},
		Hostnames:    []string{"example.com"},
		AttachedPolicies: AttachedPolicies{
			Policies: map[schema.GroupKind][]PolicyAtt{},
		},
		Rules: []HttpRouteRuleIR{{
			Name:             "rule-0",
			ExtensionRefs:    AttachedPolicies{Policies: map[schema.GroupKind][]PolicyAtt{}},
			AttachedPolicies: AttachedPolicies{Policies: map[schema.GroupKind][]PolicyAtt{}},
			Backends: []HttpBackendOrDelegate{{
				AttachedPolicies: AttachedPolicies{Policies: map[schema.GroupKind][]PolicyAtt{}},
				Backend:          new(baseHarnessBackendRefIR()),
			}},
		}},
		PrecedenceWeight:               0,
		DelegationInheritParentMatcher: false,
	}
}

func TestHarnessHttpRouteIREquals(t *testing.T) {
	gk := schema.GroupKind{Group: "example.com", Kind: "MyPolicy"}

	cases := []equalstest.Case[HttpRouteIR]{
		{
			Field:  "ObjectSource",
			Mutate: func(r *HttpRouteIR) { r.ObjectSource.Name = "other-route" },
		},
		{
			// SourceObject: a spec edit bumps generation, which versionEquals compares.
			Field: "SourceObject",
			Mutate: func(r *HttpRouteIR) {
				obj := baseHarnessHTTPRouteObj()
				obj.Generation = 2
				r.SourceObject = obj
			},
		},
		{
			Field: "AttachedPolicies",
			Mutate: func(r *HttpRouteIR) {
				r.AttachedPolicies.Policies[gk] = []PolicyAtt{baseHarnessPolicyAtt()}
			},
		},
		{
			// Rules: resolved backends live here and are not covered by the source
			// object's generation, so rulesEqual must compare them.
			Field: "Rules",
			Mutate: func(r *HttpRouteIR) {
				r.Rules[0].Backends[0].Backend.ClusterName = "blackhole_cluster"
			},
		},
		{
			Field:  "PrecedenceWeight",
			Mutate: func(r *HttpRouteIR) { r.PrecedenceWeight = 10 },
		},
		{
			Field:  "DelegationInheritParentMatcher",
			Mutate: func(r *HttpRouteIR) { r.DelegationInheritParentMatcher = true },
		},
	}

	equalstest.Run(
		t,
		baseHarnessHttpRouteIR,
		func(a, b HttpRouteIR) bool { return a.Equals(b) },
		cases,
		append(routeSpecFields, embeddedObjectSourceFields...),
	)
}

// TestHarnessHttpRouteIRRulesEquals covers the rest of what rulesEqual is
// responsible for: everything in a rule that is resolved from objects other
// than the route itself, and so cannot ride on the route's generation.
func TestHarnessHttpRouteIRRulesEquals(t *testing.T) {
	gk := schema.GroupKind{Group: "example.com", Kind: "MyPolicy"}

	for _, tc := range []struct {
		name   string
		mutate func(*HttpRouteIR)
	}{
		{
			name:   "rule count",
			mutate: func(r *HttpRouteIR) { r.Rules = append(r.Rules, HttpRouteRuleIR{}) },
		},
		{
			name: "rule attached policies",
			mutate: func(r *HttpRouteIR) {
				r.Rules[0].AttachedPolicies.Policies[gk] = []PolicyAtt{baseHarnessPolicyAtt()}
			},
		},
		{
			name: "rule extension refs",
			mutate: func(r *HttpRouteIR) {
				r.Rules[0].ExtensionRefs.Policies[gk] = []PolicyAtt{baseHarnessPolicyAtt()}
			},
		},
		{
			name:   "backend count",
			mutate: func(r *HttpRouteIR) { r.Rules[0].Backends = nil },
		},
		{
			name: "backend policies",
			mutate: func(r *HttpRouteIR) {
				r.Rules[0].Backends[0].AttachedPolicies.Policies[gk] = []PolicyAtt{baseHarnessPolicyAtt()}
			},
		},
		{
			name:   "rule error",
			mutate: func(r *HttpRouteIR) { r.Rules[0].Err = errTestRuleResolution },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			orig := baseHarnessHttpRouteIR()
			mutated := baseHarnessHttpRouteIR()
			tc.mutate(&mutated)

			if orig.Equals(mutated) {
				t.Errorf("Equals returned true after changing %s; rulesEqual must detect it", tc.name)
			}
		})
	}
}

// TestRouteIREqualsObservesSpecChangeViaGeneration is the evidence for the
// +noKrtEquals markers on ParentRefs and Hostnames: the collections that build
// these IRs copy both fields straight out of the spec, and the API server bumps
// generation on any spec write, so versionEquals catches the change.
func TestRouteIREqualsObservesSpecChangeViaGeneration(t *testing.T) {
	t.Run("HttpRouteIR", func(t *testing.T) {
		orig := baseHarnessHttpRouteIR()
		// Model what transformHttpRoute produces after a spec edit: new hostnames and
		// parentRefs, on an object whose generation has been bumped.
		updated := baseHarnessHttpRouteIR()
		obj := baseHarnessHTTPRouteObj()
		obj.Generation = 2
		obj.Spec.Hostnames = []gwv1.Hostname{"other.example.com"}
		updated.SourceObject = obj
		updated.Hostnames = []string{"other.example.com"}
		updated.ParentRefs = []gwv1.ParentReference{{Name: "other-gateway"}}

		if orig.Equals(updated) {
			t.Error("Equals returned true for a route whose spec (and generation) changed")
		}
	})

	t.Run("TlsRouteIR", func(t *testing.T) {
		orig := baseHarnessTlsRouteIR()
		updated := baseHarnessTlsRouteIR()
		obj := baseHarnessTLSRouteObj()
		obj.Generation = 2
		updated.SourceObject = obj
		updated.Hostnames = []string{"other.example.com"}
		updated.ParentRefs = []gwv1.ParentReference{{Name: "other-gateway"}}

		if orig.Equals(updated) {
			t.Error("Equals returned true for a route whose spec (and generation) changed")
		}
	})

	t.Run("TcpRouteIR", func(t *testing.T) {
		orig := baseHarnessTcpRouteIR()
		updated := baseHarnessTcpRouteIR()
		obj := baseHarnessTCPRouteObj()
		obj.Generation = 2
		updated.SourceObject = obj
		updated.ParentRefs = []gwv1.ParentReference{{Name: "other-gateway"}}

		if orig.Equals(updated) {
			t.Error("Equals returned true for a route whose spec (and generation) changed")
		}
	})
}

func baseHarnessTCPRouteObj() *gwv1a2.TCPRoute {
	return &gwv1a2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-tcp-route",
			Namespace:  "default",
			UID:        "tcp-route-uid-1",
			Generation: 1,
		},
	}
}

func baseHarnessTcpRouteIR() TcpRouteIR {
	return TcpRouteIR{
		ObjectSource: ObjectSource{
			Group:     gwv1a2.GroupVersion.Group,
			Kind:      "TCPRoute",
			Namespace: "default",
			Name:      "my-tcp-route",
		},
		SourceObject:     baseHarnessTCPRouteObj(),
		ParentRefs:       []gwv1.ParentReference{{Name: "my-gateway"}},
		AttachedPolicies: AttachedPolicies{Policies: map[schema.GroupKind][]PolicyAtt{}},
		Backends:         []BackendRefIR{baseHarnessBackendRefIR()},
	}
}

func TestHarnessTcpRouteIREquals(t *testing.T) {
	gk := schema.GroupKind{Group: "example.com", Kind: "MyPolicy"}

	cases := []equalstest.Case[TcpRouteIR]{
		{
			Field:  "ObjectSource",
			Mutate: func(r *TcpRouteIR) { r.ObjectSource.Name = "other-route" },
		},
		{
			Field: "SourceObject",
			Mutate: func(r *TcpRouteIR) {
				obj := baseHarnessTCPRouteObj()
				obj.Generation = 2
				r.SourceObject = obj
			},
		},
		{
			Field: "AttachedPolicies",
			Mutate: func(r *TcpRouteIR) {
				r.AttachedPolicies.Policies[gk] = []PolicyAtt{baseHarnessPolicyAtt()}
			},
		},
		{
			Field:  "Backends",
			Mutate: func(r *TcpRouteIR) { r.Backends[0].ClusterName = "blackhole_cluster" },
		},
	}

	equalstest.Run(
		t,
		baseHarnessTcpRouteIR,
		func(a, b TcpRouteIR) bool { return a.Equals(b) },
		cases,
		append([]string{"ParentRefs"}, embeddedObjectSourceFields...),
	)
}

func baseHarnessTLSRouteObj() *gwv1a2.TLSRoute {
	return &gwv1a2.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-tls-route",
			Namespace:  "default",
			UID:        "tls-route-uid-1",
			Generation: 1,
		},
		Spec: gwv1a2.TLSRouteSpec{
			Hostnames: []gwv1.Hostname{"example.com"},
		},
	}
}

func baseHarnessTlsRouteIR() TlsRouteIR {
	return TlsRouteIR{
		ObjectSource: ObjectSource{
			Group:     gwv1a2.GroupVersion.Group,
			Kind:      "TLSRoute",
			Namespace: "default",
			Name:      "my-tls-route",
		},
		SourceObject:     baseHarnessTLSRouteObj(),
		ParentRefs:       []gwv1.ParentReference{{Name: "my-gateway"}},
		Hostnames:        []string{"example.com"},
		AttachedPolicies: AttachedPolicies{Policies: map[schema.GroupKind][]PolicyAtt{}},
		Backends:         []BackendRefIR{baseHarnessBackendRefIR()},
	}
}

func TestHarnessTlsRouteIREquals(t *testing.T) {
	gk := schema.GroupKind{Group: "example.com", Kind: "MyPolicy"}

	cases := []equalstest.Case[TlsRouteIR]{
		{
			Field:  "ObjectSource",
			Mutate: func(r *TlsRouteIR) { r.ObjectSource.Name = "other-route" },
		},
		{
			Field: "SourceObject",
			Mutate: func(r *TlsRouteIR) {
				obj := baseHarnessTLSRouteObj()
				obj.Generation = 2
				r.SourceObject = obj
			},
		},
		{
			Field: "AttachedPolicies",
			Mutate: func(r *TlsRouteIR) {
				r.AttachedPolicies.Policies[gk] = []PolicyAtt{baseHarnessPolicyAtt()}
			},
		},
		{
			Field:  "Backends",
			Mutate: func(r *TlsRouteIR) { r.Backends[0].ClusterName = "blackhole_cluster" },
		},
	}

	equalstest.Run(
		t,
		baseHarnessTlsRouteIR,
		func(a, b TlsRouteIR) bool { return a.Equals(b) },
		cases,
		append(routeSpecFields, embeddedObjectSourceFields...),
	)
}

// MARK: policy wrapper

func baseHarnessPolicyObj() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-policy",
			Namespace:  "default",
			UID:        "policy-uid-1",
			Generation: 1,
		},
	}
}

func baseHarnessPolicyWrapper() PolicyWrapper {
	return PolicyWrapper{
		ObjectSource: ObjectSource{
			Group:     "gateway.kgateway.dev",
			Kind:      "TrafficPolicy",
			Namespace: "default",
			Name:      "my-policy",
		},
		Policy:           baseHarnessPolicyObj(),
		Errors:           nil,
		PolicyIR:         &harnessTestPolicyIR{val: "base"},
		TargetRefs:       []PolicyRef{{Group: gwv1.GroupName, Kind: "HTTPRoute", Name: "my-route"}},
		PrecedenceWeight: 0,
	}
}

func TestHarnessPolicyWrapperEquals(t *testing.T) {
	cases := []equalstest.Case[PolicyWrapper]{
		{
			Field:  "ObjectSource",
			Mutate: func(p *PolicyWrapper) { p.ObjectSource.Name = "other-policy" },
		},
		{
			Field: "Policy",
			Mutate: func(p *PolicyWrapper) {
				obj := baseHarnessPolicyObj()
				obj.Generation = 2
				p.Policy = obj
			},
		},
		{
			Field:  "Errors",
			Mutate: func(p *PolicyWrapper) { p.Errors = []error{errTestRuleResolution} },
		},
		{
			Field:  "PolicyIR",
			Mutate: func(p *PolicyWrapper) { p.PolicyIR = &harnessTestPolicyIR{val: "changed"} },
		},
		{
			Field:  "PrecedenceWeight",
			Mutate: func(p *PolicyWrapper) { p.PrecedenceWeight = 7 },
		},
	}

	equalstest.Run(
		t,
		baseHarnessPolicyWrapper,
		func(a, b PolicyWrapper) bool { return a.Equals(b) },
		cases,
		// TargetRefs is derived from the policy spec's targetRefs/targetSelectors, so a
		// change to it arrives with a generation bump; see the "Policy" case above and
		// TestPolicyWrapperEqualsObservesTargetRefChangeViaGeneration.
		append([]string{"TargetRefs"}, embeddedObjectSourceFields...),
	)
}

// TestPolicyWrapperEqualsObservesTargetRefChangeViaGeneration is the evidence for
// the +noKrtEquals marker on TargetRefs for policies whose targets come from
// a CRD spec: editing that spec bumps generation. Annotation-derived targets
// use the metadata/resourceVersion checks in versionEquals instead.
func TestPolicyWrapperEqualsObservesTargetRefChangeViaGeneration(t *testing.T) {
	orig := baseHarnessPolicyWrapper()

	updated := baseHarnessPolicyWrapper()
	obj := baseHarnessPolicyObj()
	obj.Generation = 2
	updated.Policy = obj
	updated.TargetRefs = []PolicyRef{{Group: gwv1.GroupName, Kind: "HTTPRoute", Name: "other-route"}}

	if orig.Equals(updated) {
		t.Error("Equals returned true for a policy whose targetRefs (and generation) changed")
	}
}

// MARK: secret

func baseHarnessSecretObj() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default",
			UID:       "secret-uid-1",
			// Secrets carry no generation, so versionEquals compares resourceVersion.
			ResourceVersion: "1",
		},
		Data: map[string][]byte{"tls.key": []byte("original")},
	}
}

func baseHarnessSecret() Secret {
	obj := baseHarnessSecretObj()
	return Secret{
		ObjectSource: ObjectSource{
			Kind:      "Secret",
			Namespace: "default",
			Name:      "my-secret",
		},
		Obj:  obj,
		Data: obj.Data,
	}
}

func TestHarnessSecretEquals(t *testing.T) {
	cases := []equalstest.Case[Secret]{
		{
			Field:  "ObjectSource",
			Mutate: func(s *Secret) { s.ObjectSource.Name = "other-secret" },
		},
		{
			Field: "Obj",
			Mutate: func(s *Secret) {
				obj := baseHarnessSecretObj()
				obj.ResourceVersion = "2"
				s.Obj = obj
			},
		},
	}

	equalstest.Run(
		t,
		baseHarnessSecret,
		func(a, b Secret) bool { return a.Equals(b) },
		cases,
		// Data is a verbatim copy of Obj's data; see TestSecretEqualsObservesDataChange.
		append([]string{"Data"}, embeddedObjectSourceFields...),
	)
}

// TestSecretEqualsObservesDataChange is the evidence for the +noKrtEquals marker
// on Secret.Data: the API server bumps resourceVersion on every data write, and
// versionEquals compares resourceVersion because Secrets have no generation.
func TestSecretEqualsObservesDataChange(t *testing.T) {
	orig := baseHarnessSecret()

	// Model a rotated secret as the informer delivers it: new data, new resourceVersion.
	rotatedObj := baseHarnessSecretObj()
	rotatedObj.ResourceVersion = "2"
	rotatedObj.Data = map[string][]byte{"tls.key": []byte("rotated")}
	rotated := Secret{ObjectSource: orig.ObjectSource, Obj: rotatedObj, Data: rotatedObj.Data}

	if orig.Equals(rotated) {
		t.Error("Equals returned true for a secret whose data was rotated")
	}
}

// MARK: endpoints

func harnessBackendForEndpoints(labels map[string]string) BackendObjectIR {
	backend := NewBackendObjectIR(ObjectSource{
		Kind:      "Service",
		Namespace: "default",
		Name:      "my-service",
	}, 8080, "", "")
	backend.Obj = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "my-service",
			Namespace:       "default",
			UID:             "svc-uid-1",
			ResourceVersion: "1",
			Labels:          labels,
		},
	}
	backend.CanonicalHostname = "my-service.default.svc.cluster.local"
	backend.TrafficDistribution = wellknown.TrafficDistributionAny
	return backend
}

func harnessEndpoint(address string) EndpointWithMd {
	return EndpointWithMd{
		LbEndpoint: &envoyendpointv3.LbEndpoint{
			HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
				Endpoint: &envoyendpointv3.Endpoint{
					Hostname: address,
				},
			},
		},
		EndpointMd: EndpointMetadata{Labels: map[string]string{"pod": address}},
	}
}

func baseHarnessEndpointsForBackend() EndpointsForBackend {
	eps := NewEndpointsForBackend(harnessBackendForEndpoints(map[string]string{
		"app":     "my-app",
		"version": "v1",
		"tier":    "backend",
	}))
	eps.Add(PodLocality{Region: "us-east-1", Zone: "us-east-1a"}, harnessEndpoint("10.0.0.1"))
	return *eps
}

func TestHarnessEndpointsForBackendEquals(t *testing.T) {
	cases := []equalstest.Case[EndpointsForBackend]{
		{
			Field:  "ClusterName",
			Mutate: func(e *EndpointsForBackend) { e.ClusterName = "other_cluster" },
		},
		{
			Field:  "UpstreamResourceName",
			Mutate: func(e *EndpointsForBackend) { e.UpstreamResourceName = "other-upstream" },
		},
		{
			Field:  "Port",
			Mutate: func(e *EndpointsForBackend) { e.Port = 9090 },
		},
		{
			Field:  "Hostname",
			Mutate: func(e *EndpointsForBackend) { e.Hostname = "other.svc.cluster.local" },
		},
		{
			Field:  "TrafficDistribution",
			Mutate: func(e *EndpointsForBackend) { e.TrafficDistribution = wellknown.TrafficDistributionPreferSameZone },
		},
		{
			Field:  "LbEpsEqualityHash",
			Mutate: func(e *EndpointsForBackend) { e.LbEpsEqualityHash++ },
		},
		{
			// BackendLabels is +noKrtEquals: NewEndpointsForBackend folds the labels into
			// upstreamHash, so rebuilding from a relabelled backend must not compare equal.
			Field: "BackendLabels",
			Mutate: func(e *EndpointsForBackend) {
				rebuilt := NewEndpointsForBackend(harnessBackendForEndpoints(map[string]string{
					"app":     "my-app",
					"version": "v2",
					"tier":    "backend",
				}))
				rebuilt.Add(PodLocality{Region: "us-east-1", Zone: "us-east-1a"}, harnessEndpoint("10.0.0.1"))
				*e = *rebuilt
			},
		},
		{
			// LbEps is +noKrtEquals: Add() maintains epsEqualityHash, which is compared.
			Field: "LbEps",
			Mutate: func(e *EndpointsForBackend) {
				e.Add(PodLocality{Region: "us-east-1", Zone: "us-east-1b"}, harnessEndpoint("10.0.0.2"))
			},
		},
	}

	equalstest.Run(
		t,
		baseHarnessEndpointsForBackend,
		func(a, b EndpointsForBackend) bool { return a.Equals(b) },
		cases,
		// AttachedPolicies is +noKrtEquals. This fixture does not model policy
		// versioning; newFinalBackendEndpoints folds it into LbEpsEqualityHash.
		[]string{"AttachedPolicies"},
	)
}

// TestHarnessEndpointsForBackendEqualityHashIsDeterministic checks that rebuilding
// identical backend labels preserves equality despite randomized map iteration.
func TestHarnessEndpointsForBackendEqualityHashIsDeterministic(t *testing.T) {
	labels := map[string]string{
		"a": "1", "b": "2", "c": "3", "d": "4", "e": "5",
		"f": "6", "g": "7", "h": "8", "i": "9", "j": "10",
	}

	first := NewEndpointsForBackend(harnessBackendForEndpoints(labels))
	for range 100 {
		next := NewEndpointsForBackend(harnessBackendForEndpoints(labels))
		if !first.Equals(*next) {
			t.Fatalf("two identical EndpointsForBackend compared unequal: hash %d vs %d",
				first.LbEpsEqualityHash, next.LbEpsEqualityHash)
		}
	}
}

// TestEndpointsForBackendEmptyCopyEquality documents that EmptyCopy drops the
// endpoints (and so must not compare equal to a populated set) while preserving
// the backend identity hash.
func TestEndpointsForBackendEmptyCopyEquality(t *testing.T) {
	populated := baseHarnessEndpointsForBackend()

	empty := populated.EmptyCopy()
	if populated.Equals(empty) {
		t.Error("Equals returned true comparing a populated EndpointsForBackend to its EmptyCopy")
	}

	// Two EmptyCopies of the same backend must still be equal.
	if !empty.Equals(populated.EmptyCopy()) {
		t.Error("two EmptyCopy results for the same backend compared unequal")
	}
}
